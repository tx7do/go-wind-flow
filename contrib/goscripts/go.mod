module github.com/tx7do/go-wind-flow/contrib/goscripts

go 1.25.0

require (
	github.com/tx7do/go-scripts v0.0.8
	github.com/tx7do/go-scripts/expr v0.0.4
	github.com/tx7do/go-scripts/javascript v0.0.10
	github.com/tx7do/go-wind-flow v0.0.0
)

require (
	github.com/dlclark/regexp2/v2 v2.5.2 // indirect
	github.com/dop251/goja v0.0.0-20260723142020-b4aef50fa347 // indirect
	github.com/dop251/goja_nodejs v0.0.0-20260212111938-1f56ff5bcf14 // indirect
	github.com/expr-lang/expr v1.17.8 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/google/pprof v0.0.0-20260709232956-b9395ee17fa0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

// 本地开发替换：go-scripts 未发布版本前指向同级仓库。
// go-scripts 推送 GitHub 并打 tag 后可删除本 replace。
replace github.com/tx7do/go-scripts => ../../../go-scripts

replace github.com/tx7do/go-wind-flow => ../../
