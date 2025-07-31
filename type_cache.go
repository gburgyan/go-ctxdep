package ctxdep

import (
	"reflect"
	"sync"
)

// typeInfo caches expensive reflection operations for a type
type typeInfo struct {
	assignableToError bool
	implementsKeyable bool
	keyableMethod     reflect.Method
	keyableFound      bool
	isInterface       bool

	// For function types, cache parameter and return types
	funcParams  []reflect.Type
	funcReturns []reflect.Type
	hasError    bool
}

// Global type cache to avoid repeated reflection operations
var (
	globalTypeCache sync.Map // map[reflect.Type]*typeInfo
	keyableType     = reflect.TypeOf((*Keyable)(nil)).Elem()
)

// getTypeInfo returns cached type information, computing it if necessary
func getTypeInfo(t reflect.Type) *typeInfo {
	if cached, ok := globalTypeCache.Load(t); ok {
		return cached.(*typeInfo)
	}

	info := &typeInfo{
		assignableToError: t.AssignableTo(errorType),
		implementsKeyable: t.Implements(keyableType),
		isInterface:       t.Kind() == reflect.Interface,
	}

	// Cache Keyable method if it exists
	if info.implementsKeyable {
		info.keyableMethod, info.keyableFound = t.MethodByName("CacheKey")
	}

	// For function types, cache parameter and return information
	if t.Kind() == reflect.Func {
		info.funcParams = make([]reflect.Type, t.NumIn())
		for i := 0; i < t.NumIn(); i++ {
			info.funcParams[i] = t.In(i)
		}

		info.funcReturns = make([]reflect.Type, 0, t.NumOut())
		errorCount := 0
		for i := 0; i < t.NumOut(); i++ {
			returnType := t.Out(i)
			if returnType.AssignableTo(errorType) {
				errorCount++
				if errorCount > 1 {
					panic("multiple error results on a generator function not permitted")
				}
				info.hasError = true
			} else {
				info.funcReturns = append(info.funcReturns, returnType)
			}
		}
	}

	// Store in cache
	actual, _ := globalTypeCache.LoadOrStore(t, info)
	return actual.(*typeInfo)
}

// interfaceCache caches which concrete types implement which interfaces
type interfaceCache struct {
	mu    sync.RWMutex
	cache map[interfaceCacheKey]bool
}

type interfaceCacheKey struct {
	concrete reflect.Type
	iface    reflect.Type
}

var globalInterfaceCache = &interfaceCache{
	cache: make(map[interfaceCacheKey]bool),
}

// canAssign checks if concrete type can be assigned to interface type, with caching
func canAssign(concrete, iface reflect.Type) bool {
	if iface.Kind() != reflect.Interface {
		return concrete == iface
	}

	key := interfaceCacheKey{concrete: concrete, iface: iface}

	// Fast path: check cache
	globalInterfaceCache.mu.RLock()
	if result, ok := globalInterfaceCache.cache[key]; ok {
		globalInterfaceCache.mu.RUnlock()
		return result
	}
	globalInterfaceCache.mu.RUnlock()

	// Slow path: compute and cache
	result := concrete.AssignableTo(iface)

	globalInterfaceCache.mu.Lock()
	globalInterfaceCache.cache[key] = result
	globalInterfaceCache.mu.Unlock()

	return result
}
