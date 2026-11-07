// Disposable network qualification server; never packaged with the product.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

func main() {
	if len(os.Args) == 5 && os.Args[1] == "client" {
		root, err := base64.StdEncoding.Strict().DecodeString(os.Args[4])
		if err != nil {
			panic(err)
		}
		certificate, err := x509.ParseCertificate(root)
		if err != nil {
			panic(err)
		}
		pool := x509.NewCertPool()
		pool.AddCert(certificate)
		transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, os.Args[3])
		}}
		client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
		response, err := client.Get(os.Args[2])
		if err != nil {
			panic(err)
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			panic(response.Status)
		}
		if _, err = io.Copy(os.Stdout, io.LimitReader(response.Body, 1024)); err != nil {
			panic(err)
		}
		return
	}
	http.HandleFunc("/idle", func(w http.ResponseWriter, request *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-request.Context().Done()
	})
	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "orbwin-p2-loopback-1792892200") })
	if err := http.ListenAndServe(":39081", nil); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
