package ptr

// To 返回 v 的指针，适用于任意类型。
//
// 用法：
//
//	ptr.To(42)        // *int
//	ptr.To("hello")   // *string
//	ptr.To(true)      // *bool
func To[T any](v T) *T {
	return &v
}

// Deref 安全地解引用指针，p 为 nil 时返回 T 的零值。
//
// 用法：
//
//	ptr.Deref(strPtr)   // string，nil 时返回 ""
//	ptr.Deref(intPtr)   // int，nil 时返回 0
func Deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}
