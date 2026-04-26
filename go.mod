module github.com/chinese-room-solutions/mass-sdk

go 1.25.0

require (
	github.com/KernelPryanic/ctxerr v1.0.0
	github.com/chinese-room-solutions/mass-client-go v0.0.0
	github.com/hashicorp/go-plugin v1.7.0
	github.com/jchv/go-webview2 v0.0.0-20260205173254-56598839c808
	github.com/stretchr/testify v1.11.1
	github.com/webview/webview_go v0.0.0-20240831120633-6173450d4dd6
	golang.org/x/sys v0.42.0
	google.golang.org/grpc v1.79.1
	google.golang.org/protobuf v1.36.11
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/chinese-room-solutions/mass-proto/gen/go v0.0.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/oklog/run v1.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
)

replace github.com/chinese-room-solutions/mass-client-go => ../mass-client-go

replace github.com/chinese-room-solutions/mass-proto/gen/go => ../mass-proto/gen/go
