//go:build linux

// Disposable qualification fixture only; never packaged with the product.
package main

import (
 "fmt"
 "os"
 "syscall"
)

func main(){
 var err error
 const target="/data/project/例.txt"
 if len(os.Args)!=2 {os.Exit(2)}
 switch os.Args[1]{
 case "add":err=syscall.Setxattr(target,"user.orbwin-proof",[]byte("must-not-drop"),0)
 case "remove":err=syscall.Removexattr(target,"user.orbwin-proof")
 default:os.Exit(2)
 }
 if err!=nil {fmt.Fprintln(os.Stderr,err);os.Exit(1)}
}
