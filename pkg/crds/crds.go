package crds

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"io"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/service-provider-velero/pkg/resources"
)

var (
	//go:embed crds.yaml
	crdsFile []byte
)

// Parse reads a set of embedded CRDs.
func Parse() ([]*apiextv1.CustomResourceDefinition, error) {
	crds := []*apiextv1.CustomResourceDefinition{}
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(crdsFile), 100)

	for {
		crd := &apiextv1.CustomResourceDefinition{}

		err := decoder.Decode(crd)
		if errors.Is(err, io.EOF) {
			return crds, nil
		}
		if err != nil {
			return nil, err
		}

		if crd.Name == "" {
			continue
		}
		crds = append(crds, crd)
	}
}

// Configure adds a set of managed CRD objects to the given cluster.
func Configure(cluster resources.ManagedCluster) error {
	crds, err := Parse()
	if err != nil {
		return err
	}

	for _, desired := range crds {
		crd := resources.NewManagedObject(&apiextv1.CustomResourceDefinition{
			ObjectMeta: v1.ObjectMeta{
				Name: desired.Name,
			},
		}, resources.ManagedObjectContext{
			ReconcileFunc: func(_ context.Context, o client.Object) error {
				oCRD := o.(*apiextv1.CustomResourceDefinition)
				oCRD.Spec = desired.Spec
				return nil
			},
			// orphan CRDs to prevent deleting end user data
			DeletionPolicy: resources.Orphan,
			StatusFunc:     resources.SimpleStatus,
		})
		cluster.AddObject(crd)
	}
	return nil
}
