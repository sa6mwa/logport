package logport_test

import (
	"io"

	logport "pkt.systems/logport"
	charm "pkt.systems/logport/adapters/charmlogger"
	onelog "pkt.systems/logport/adapters/onelogger"
	phuslu "pkt.systems/logport/adapters/phuslu"
	zapadapter "pkt.systems/logport/adapters/zaplogger"
	zeroadapter "pkt.systems/logport/adapters/zerologger"
)

type adapterFactory struct {
	name string
	make func(io.Writer) logport.ForLogging
}

func adapterFactories() []adapterFactory {
	return []adapterFactory{
		{name: "zerolog/console", make: func(w io.Writer) logport.ForLogging { return zeroadapter.New(w) }},
		{name: "zerolog/json", make: func(w io.Writer) logport.ForLogging { return zeroadapter.NewStructured(w) }},
		{name: "charm/console", make: func(w io.Writer) logport.ForLogging { return charm.New(w) }},
		{name: "charm/json", make: func(w io.Writer) logport.ForLogging { return charm.NewStructured(w) }},
		{name: "phuslu", make: func(w io.Writer) logport.ForLogging { return phuslu.New(w) }},
		{name: "zap", make: func(w io.Writer) logport.ForLogging { return zapadapter.New(w) }},
		{name: "onelog", make: func(w io.Writer) logport.ForLogging { return onelog.New(w) }},
	}
}
