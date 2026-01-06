package resources

import (
	"strings"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewManagedCluster(c client.Client, cfg *rest.Config, defaultNamespace string) ManagedCluster {
	return &managedCluster{
		client:           c,
		cfg:              cfg,
		objects:          []ManagedObject{},
		defaultNamespace: defaultNamespace,
	}
}

type ManagedCluster interface {
	AddObject(o ManagedObject)
	GetObjects() []ManagedObject
	GetDefaultNamespace() string
	GetHostAndPort() (string, string)
	GetConfig() *rest.Config
	GetClient() client.Client
}

var _ ManagedCluster = &managedCluster{}

type managedCluster struct {
	client           client.Client
	cfg              *rest.Config
	objects          []ManagedObject
	defaultNamespace string
}

// GetClient implements ManagedCluster.
func (m *managedCluster) GetClient() client.Client {
	return m.client
}

// GetConfig implements ManagedCluster.
func (m *managedCluster) GetConfig() *rest.Config {
	return m.cfg
}

// GetHostAndPort implements ManagedCluster.
func (m *managedCluster) GetHostAndPort() (string, string) {
	input := strings.TrimPrefix(m.cfg.Host, "https://")
	host, port, found := strings.Cut(input, ":")
	if !found {
		port = "443"
	}
	return host, port
}

// GetDefaultNamespace implements ManagedCluster.
func (m *managedCluster) GetDefaultNamespace() string {
	return m.defaultNamespace
}

// AddObject implements ManagedCluster.
func (m *managedCluster) AddObject(o ManagedObject) {
	m.objects = append(m.objects, o)
}

// GetObjects implements ManagedCluster.
func (m *managedCluster) GetObjects() []ManagedObject {
	return m.objects
}
