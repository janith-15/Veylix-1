// Package config lives without tzdata on a slim host unless this file is here.
//
// The server binary imports time/tzdata for its side effect (see
// cmd/veylixd/main.go). This package's tests assert real Colombo offsets
// against time.LoadLocation, so without the same import they inherit whatever
// tzdata the TEST HOST happens to have. That is the difference between "works
// on the developer's Windows box" and "red on the Linux runner", and it is a
// property of the test, not of the product — so it is fixed here rather than
// by weakening the assertions, which are the thing worth keeping.
package config

import _ "time/tzdata"
