package schemes

import (
	apiv1alpha1 "github.com/openmcp-project/service-provider-velero/api/v1alpha1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

var (
	Workload = runtime.NewScheme()
	Mcp      = runtime.NewScheme()
)

func init() {
	// Local
	utilruntime.Must(clientgoscheme.AddToScheme(Workload))
	utilruntime.Must(apiv1alpha1.AddToScheme(Workload))

	// Remote
	utilruntime.Must(clientgoscheme.AddToScheme(Mcp))
	utilruntime.Must(apiextv1.AddToScheme(Mcp))
}
