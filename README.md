[![GoDoc](https://godoc.org/github.com/EnumApps/clustersqlstore?status.svg)](http://godoc.org/github.com/EnumApps/clustersqlstore)

clustersqlstore
==========

Gorilla's Session Store Implementation for ClusterSQL - Server Fram of MySQLs

Dependency
===========

EnumApps/clustersql (https://github.com/EnumApps/clustersql) 
Run 

      go get github.com/go-sql-driver/mysql 
      go get github.com/EnumApps/clustersqlstore
      go get github.com/EnumApps/aerror


Installation
===========

Run  

      go get github.com/EnumApps/clustersqlstore

from command line. Gets installed in `$GOPATH`

Usage
=====


`NewClusterSQLStore` takes the following paramaters

    driverName  - the name of pre-registered cluster drvier, see EnumApps/clustersql (https://github.com/EnumApps/clustersql) for detail
    path        - path for Set-Cookie header
    maxAge      - maxAge for session
    codecs

Internally, `clustersqlstore` uses [this](https://github.com/EnumApps/clustersql) ClusterSQL driver (not the original one; Since you may using another connection for your major data, this patched version allow multi drivers, each have a custom name).

e.g.,
      

      package main
  
      import (
  	    "fmt"
  	    "github.com/EnumApps/clustersqlstore"
  	    "net/http"
      )
  
      var store, _ = clustersqlstore.NewClusterSQLStore(<drivername>, "/", 3600, []byte("<SecretKey>"))
      defer store.Close()
  
      func sessTest(w http.ResponseWriter, r *http.Request) {
  	    session, err := store.Get(r, "foobar")
  	    session.Values["bar"] = "baz"
  	    session.Values["baz"] = "foo"
  	    err = session.Save(r, w)
  	    fmt.Printf("%#v\n", session)
  	    fmt.Println(err)
      }

    func main() {
    	http.HandleFunc("/", sessTest)
    	http.ListenAndServe(":8080", nil)
    }
