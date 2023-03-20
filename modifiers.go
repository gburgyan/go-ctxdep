package ctxdep

type GeneratorLogger interface {
	CallGenerator(typeName string, toCall func())
}
