// Static, shell-less Linux test target. Only disposable proof containers use it.
package main

import (
	"fmt"
	"net/http"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintln(w, "orbwin-debug-target") })
	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}
