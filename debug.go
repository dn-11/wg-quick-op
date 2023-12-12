//go:build debug

package main

import (
	"net/http"
	_ "net/http/pprof"
)

func init() {
	go http.ListenAndServe(":46789", nil)
}
