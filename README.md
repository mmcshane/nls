# NLS is Non-lexical Scopes for Go

[![GoDoc](https://godoc.org/github.com/mmcshane/nls?status.svg)](https://godoc.org/github.com/mmcshane/nls)

Go has `defer` which gives something approaching strict lexical lifetimes for
resource management but there is no direct support for object or resource
lifetimes that may outlive the function in which they are created. This small
library allows the user to create a tree of scope instances, each of which owns
the lifetime of zero or more resource-consuming objects (e.g. goroutines,
sockets, gRPC clients, etc) and which can be used to cleanly release said
resources.
