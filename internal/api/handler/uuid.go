package handler

import "reflect"

// getUUID extracts the UUID field from any OVSDB model struct.
// All generated models have a UUID string field tagged `ovsdb:"_uuid"`.
func getUUID(model any) string {
	v := reflect.ValueOf(model)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	f := v.FieldByName("UUID")
	if !f.IsValid() {
		return ""
	}
	return f.String()
}
