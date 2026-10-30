package eval

import "reflect"

// sameRef reports whether two arrays or objects are the same instance, which is when GitHub says they are equal.
func sameRef(a, b any) bool {
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	if va.Kind() != vb.Kind() || (va.Kind() != reflect.Map && va.Kind() != reflect.Slice) {
		return false
	}
	return va.Pointer() == vb.Pointer() && (va.Kind() == reflect.Map || va.Len() == vb.Len())
}
