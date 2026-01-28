package instance

import (
	"crypto/sha1"
	"encoding/base32"
	"fmt"

	"github.com/openmcp-project/service-provider-velero/api/v1alpha1"
)

const (
	labelInstanceID          = "velero.services.openmcp.cloud/instance-id"
	base32EncodeStdLowerCase = "abcdefghijklmnopqrstuvwxyz234567"
)

func GetID(o *v1alpha1.Velero) string {
	return o.Labels[labelInstanceID]
}

func SetID(o *v1alpha1.Velero, tenantID string) {
	if o.Labels == nil {
		o.Labels = map[string]string{}
	}
	o.Labels[labelInstanceID] = tenantID
}

func GenerateID(o *v1alpha1.Velero) string {
	h := sha1.New()
	fmt.Fprintf(h, "%s/%s", o.Namespace, o.Name)
	id := base32.NewEncoding(base32EncodeStdLowerCase).WithPadding(base32.NoPadding).EncodeToString(h.Sum(nil))
	return id
}

func Namespace(o *v1alpha1.Velero) string {
	return fmt.Sprintf("velero-%s", GetID(o))
}
