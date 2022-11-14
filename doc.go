// Package ctxdep provides a convenient way to add dependencies to a context. It has the ability to have
// simple objects in the context to be asked for by type as well as objects that implement an interface to
// be retrieved by the interface type. In addition, it supports generator functions that can lazily create
// either.
//
// The DependencyContext object has comprehensive documentation about how it works.
//
// There are also helper global functions that make using this more concise.
package ctxdep
