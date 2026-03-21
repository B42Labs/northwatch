package write

import (
	"fmt"
	"reflect"
	"strings"
)

// MapToModel converts a JSON map (keyed by ovsdb tag names) into a Go model struct.
// It is the inverse of api.ModelToMap.
func MapToModel(fields map[string]any, modelType reflect.Type) (any, error) {
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}

	val := reflect.New(modelType).Elem()

	// Build a lookup from ovsdb tag -> struct field index.
	tagIndex := make(map[string]int)
	for i := 0; i < modelType.NumField(); i++ {
		tag := modelType.Field(i).Tag.Get("ovsdb")
		if tag == "" || tag == "-" {
			continue
		}
		if idx := strings.Index(tag, ","); idx != -1 {
			tag = tag[:idx]
		}
		tagIndex[tag] = i
	}

	for key, rawVal := range fields {
		idx, ok := tagIndex[key]
		if !ok {
			return nil, fmt.Errorf("unknown field %q", key)
		}
		field := val.Field(idx)
		if err := setField(field, rawVal); err != nil {
			return nil, fmt.Errorf("field %q: %w", key, err)
		}
	}

	result := val.Addr().Interface()
	return result, nil
}

// setField assigns rawVal to a reflect.Value, performing necessary type conversions.
func setField(field reflect.Value, rawVal any) error {
	if rawVal == nil {
		return nil
	}

	fieldType := field.Type()

	// Handle pointer types: *string, *int, *bool
	if fieldType.Kind() == reflect.Ptr {
		elemType := fieldType.Elem()
		ptrVal := reflect.New(elemType)
		if err := setField(ptrVal.Elem(), rawVal); err != nil {
			return err
		}
		field.Set(ptrVal)
		return nil
	}

	switch fieldType.Kind() {
	case reflect.String:
		s, ok := rawVal.(string)
		if !ok {
			return fmt.Errorf("expected string, got %T", rawVal)
		}
		field.SetString(s)

	case reflect.Int, reflect.Int64:
		switch v := rawVal.(type) {
		case float64:
			field.SetInt(int64(v))
		case int:
			field.SetInt(int64(v))
		case int64:
			field.SetInt(v)
		default:
			return fmt.Errorf("expected number, got %T", rawVal)
		}

	case reflect.Bool:
		b, ok := rawVal.(bool)
		if !ok {
			return fmt.Errorf("expected bool, got %T", rawVal)
		}
		field.SetBool(b)

	case reflect.Slice:
		return setSliceField(field, rawVal)

	case reflect.Map:
		return setMapField(field, rawVal)

	default:
		return fmt.Errorf("unsupported field type %s", fieldType.Kind())
	}

	return nil
}

// setSliceField handles []string and similar slice types.
func setSliceField(field reflect.Value, rawVal any) error {
	rv := reflect.ValueOf(rawVal)
	if rv.Kind() != reflect.Slice {
		return fmt.Errorf("expected slice, got %T", rawVal)
	}

	elemType := field.Type().Elem()
	slice := reflect.MakeSlice(field.Type(), rv.Len(), rv.Len())

	for i := 0; i < rv.Len(); i++ {
		elem := rv.Index(i).Interface()
		target := reflect.New(elemType).Elem()
		if err := setField(target, elem); err != nil {
			return fmt.Errorf("index %d: %w", i, err)
		}
		slice.Index(i).Set(target)
	}

	field.Set(slice)
	return nil
}

// setMapField handles map[string]string, map[string]any, etc.
func setMapField(field reflect.Value, rawVal any) error {
	rv := reflect.ValueOf(rawVal)
	if rv.Kind() != reflect.Map {
		return fmt.Errorf("expected map, got %T", rawVal)
	}

	mapType := field.Type()
	newMap := reflect.MakeMap(mapType)
	valType := mapType.Elem()

	for _, key := range rv.MapKeys() {
		elemVal := rv.MapIndex(key).Interface()

		targetKey := reflect.New(mapType.Key()).Elem()
		if err := setField(targetKey, key.Interface()); err != nil {
			return fmt.Errorf("map key: %w", err)
		}

		targetVal := reflect.New(valType).Elem()
		if err := setField(targetVal, elemVal); err != nil {
			return fmt.Errorf("map value for key %v: %w", key.Interface(), err)
		}

		newMap.SetMapIndex(targetKey, targetVal)
	}

	field.Set(newMap)
	return nil
}
