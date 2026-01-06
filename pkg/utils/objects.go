package utils

import (
	"fmt"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ObjectID(obj client.Object) string {
	typeName := reflect.TypeOf(obj).Elem().Name()
	return fmt.Sprintf(`Kind=%s, Name=%s, Namespace=%s`, typeName, obj.GetName(), obj.GetNamespace())
}
