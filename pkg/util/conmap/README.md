This came about because I wanted to have concurrent maps but felt bad
about casting out of them.

To re-generate the gen-conmaps.go with updates from conmaps.go:

```bash
go get github.com/justnoise/genny
cd $GOPATH/src/github.com/justnoise/genny
go install
cd $GOPATH/src/github.com/elotl/itzo/pkg/util/conmap
go generate
```

*Note:* Don't use github.com/cheekybits/genny for code generation.
The original genny didn't handle many things well since it doesn't do
real parsing (e.g. `make([]NodeKeyTypeValueType` when
`KeyType=string`).  So use the justnoise version of genny since it has
been updated to parse the go source code.