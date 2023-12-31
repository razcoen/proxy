package proxy

var (
	_ Keyable = StringKeyable("")
	_ Keyable = StaticKeyable[any]{}
	_ Keyable = DynamicKeyable[any]{}
)

type StringKeyable string

func NewKeyableString(key string) StringKeyable {
	return StringKeyable(key)
}

func (ks StringKeyable) Key() string {
	return string(ks)
}

type StaticKeyable[T any] struct {
	key   string
	Value T
}

func NewKeyableStaticKey[T any](key string, value T) StaticKeyable[T] {
	return StaticKeyable[T]{
		key:   key,
		Value: value,
	}
}

func (ka StaticKeyable[T]) Key() string {
	return ka.key
}

type DynamicKeyable[T any] struct {
	keyFunc func(t T) string
	Value   T
}

func NewKeyableDynamicKey[T any](value T, keyFunc func(t T) string) DynamicKeyable[T] {
	return DynamicKeyable[T]{
		keyFunc: keyFunc,
		Value:   value,
	}
}

func (ka DynamicKeyable[T]) Key() string {
	return ka.keyFunc(ka.Value)
}
