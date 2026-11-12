package authn_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/openmcp-project/extensibility-utils/pkg/tokenrefresh"

	"github.com/openmcp-project/service-provider-velero/pkg/authn"
	"github.com/openmcp-project/service-provider-velero/pkg/resources"
	"github.com/openmcp-project/service-provider-velero/pkg/testutils"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

const (
	serviceAccountMountPath = "/var/run/secrets/kubernetes.io/serviceaccount"
	serviceAccountVolume    = "kube-api-access"
	testNamespace           = "test"
	secretName              = "kube-api-access-velero-server"
	expAnnotation           = "velero.services.open-control-plane.io/token-expiration-time"
	existingToken           = "existing-token"
	mintedToken             = "minted-token"
)

var caBundle = []byte("ca-bundle")

// mcpClusterWithTokenIssuer builds an MCP cluster whose fake client mints ServiceAccount tokens and
// whose REST config carries a CA bundle, so the initial token write in Configure can succeed.
func mcpClusterWithTokenIssuer(t *testing.T) *clusters.Cluster {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	cl := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
		SubResourceCreate: func(ctx context.Context, c client.Client, subResource string, obj client.Object, subResourceObj client.Object, opts ...client.SubResourceCreateOption) error {
			tr, ok := subResourceObj.(*authenticationv1.TokenRequest)
			if subResource != "token" || !ok {
				return c.SubResource(subResource).Create(ctx, obj, subResourceObj, opts...)
			}
			tr.Status = authenticationv1.TokenRequestStatus{
				Token:               mintedToken,
				ExpirationTimestamp: metav1.NewTime(time.Now().Add(time.Hour)),
			}
			return nil
		},
	}).Build()
	return clusters.NewTestClusterFromClient("mcp", cl).WithRESTConfig(&rest.Config{TLSClientConfig: rest.TLSClientConfig{CAData: caBundle}})
}

func TestManagedServiceAccount_Configure(t *testing.T) {
	existingExpirationTime := time.Now().Add(time.Hour).Format(time.RFC3339)
	existingCreationTime := metav1.NewTime(time.Now().Add(-1 * time.Minute).Truncate(time.Second))
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		workloadCluster    resources.ManagedCluster
		mcpCluster         resources.ManagedCluster
		wantSecretCreation bool
		wantErrors         []string
	}{
		{
			name:               "secret is initially created with a token",
			workloadCluster:    resources.NewManagedCluster(testutils.CreateFakeCluster(t, "workload"), &rest.Config{}, testNamespace, resources.WorkloadCluster),
			mcpCluster:         resources.NewManagedCluster(mcpClusterWithTokenIssuer(t), &rest.Config{}, testNamespace, resources.ManagedControlPlane),
			wantSecretCreation: true,
		},
		{
			name: "existing token and expiration time are kept",
			workloadCluster: resources.NewManagedCluster(testutils.CreateFakeCluster(t, "workload", &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						expAnnotation: existingExpirationTime,
					},
					CreationTimestamp: existingCreationTime,
				},
				Data: map[string][]byte{
					"token": []byte(existingToken),
				},
			},
			), &rest.Config{}, testNamespace, resources.WorkloadCluster),
			mcpCluster:         resources.NewManagedCluster(testutils.CreateFakeCluster(t, "mcp"), &rest.Config{}, testNamespace, resources.ManagedControlPlane),
			wantSecretCreation: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &authn.ManagedServiceAccount{
				NamespacedName: types.NamespacedName{
					Name:      "velero-server",
					Namespace: testNamespace,
				},
			}
			// call function under test
			tokenApplyFunc := m.Configure(tt.workloadCluster, tt.mcpCluster)
			testutils.ExecApply(t, []resources.ManagedCluster{tt.mcpCluster, tt.workloadCluster}, 2, tt.wantErrors)

			// verify service account exists
			require.Len(t, tt.mcpCluster.GetObjects(), 1)
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "velero-server",
					Namespace: testNamespace,
				},
			}
			assert.NoError(t, tt.mcpCluster.GetClient().Get(context.TODO(), client.ObjectKeyFromObject(sa), sa))

			// verify secret exists
			require.Len(t, tt.workloadCluster.GetObjects(), 1)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
			}
			require.NoError(t, tt.workloadCluster.GetClient().Get(context.TODO(), client.ObjectKeyFromObject(secret), secret))

			// assert secret creation
			gotSecretCreation := !(secret.GetCreationTimestamp().Time.Equal(existingCreationTime.Time))
			assert.Equal(t, tt.wantSecretCreation, gotSecretCreation)

			if tt.wantSecretCreation {
				// the initial token is minted by Configure so the pod never mounts an empty volume;
				// the tokenrefresh controller only renews it afterwards
				assert.Equal(t, mintedToken, string(secret.Data[tokenrefresh.SecretKeyToken]))
				assert.Equal(t, caBundle, secret.Data[tokenrefresh.SecretKeyCA])
				assert.Equal(t, testNamespace, string(secret.Data[tokenrefresh.SecretKeyNamespace]))
				assert.NotEmpty(t, secret.GetAnnotations()[expAnnotation])
				assert.NotEmpty(t, secret.GetAnnotations()[tokenrefresh.CreationAnnotation(authn.TokenServiceProvider)])
			} else {
				// an existing token must be kept: Configure must neither re-mint nor drop it
				assert.Equal(t, existingToken, string(secret.Data["token"]))
				assert.Equal(t, existingExpirationTime, secret.GetAnnotations()[expAnnotation])
			}

			// apply the to dummy pod
			dummyPodSpec := &corev1.PodSpec{
				InitContainers: []corev1.Container{
					{
						Name: "init",
					},
				},
				Containers: []corev1.Container{
					{
						Name: "velero-server",
					},
				},
			}

			// test tokenApplyFunc behavior
			tokenApplyFunc(dummyPodSpec)

			// verify token mount to be used as in cluster config
			require.Len(t, dummyPodSpec.Volumes, 1)
			assert.Equal(t, serviceAccountVolume, dummyPodSpec.Volumes[0].Name)
			assert.Equal(t, secretName, dummyPodSpec.Volumes[0].Secret.SecretName)
			require.Len(t, dummyPodSpec.Containers, 1)
			require.Len(t, dummyPodSpec.InitContainers, 1)
			require.Len(t, dummyPodSpec.Containers[0].VolumeMounts, 1)
			assert.Equal(t, serviceAccountVolume, dummyPodSpec.Containers[0].VolumeMounts[0].Name)
			assert.Equal(t, serviceAccountMountPath, dummyPodSpec.Containers[0].VolumeMounts[0].MountPath)
			require.Len(t, dummyPodSpec.InitContainers[0].VolumeMounts, 1)
			assert.Equal(t, serviceAccountVolume, dummyPodSpec.InitContainers[0].VolumeMounts[0].Name)
			assert.Equal(t, serviceAccountMountPath, dummyPodSpec.InitContainers[0].VolumeMounts[0].MountPath)
		})
	}
}

func TestManagedServiceAccount_TokenBinding(t *testing.T) {
	m := &authn.ManagedServiceAccount{
		NamespacedName: types.NamespacedName{Name: "velero-server", Namespace: "velero"},
	}
	target := m.TokenBinding("workload-ns")
	assert.Equal(t, types.NamespacedName{Name: "velero-server", Namespace: "velero"}, target.ServiceAccount)
	assert.Equal(t, types.NamespacedName{Name: secretName, Namespace: "workload-ns"}, target.Secret)
	assert.Equal(t, expAnnotation, tokenrefresh.ExpirationAnnotation(authn.TokenServiceProvider))
}
