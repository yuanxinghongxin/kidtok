// jsprobe 用 goja 尝试在 Go 里加载 TVBox 的 JS 爬虫，验证「能不能直接跑」。
//
// 用法：
//
//	go run . -engine drpy2.min.js -rule tuxiaobei.js
//
// 结论（对 drpy2 + Vue/TVBox 生态）：引擎文件是 ES module，首行 import
// "assets://js/lib/cheerio.min.js" 等资源，goja 无法解析 import，直接报语法错误；
// 规则文件是纯对象字面量，可以加载，但里面只有字符串规则模板，没有可调用的搜索函数。
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/dop251/goja"
)

// newVM 模拟 TVBox 注入的宿主函数：print 和 req。
func newVM() *goja.Runtime {
	vm := goja.New()
	vm.Set("print", func(call goja.FunctionCall) goja.Value {
		fmt.Println("[print]", call.Argument(0).String())
		return goja.Undefined()
	})
	// req(url) 是 TVBox 的同步 HTTP 抓取；这里只记录调用，不真正发请求。
	vm.Set("req", func(call goja.FunctionCall) goja.Value {
		fmt.Println("[req]", call.Argument(0).String())
		return vm.ToValue("")
	})
	return vm
}

func main() {
	enginePath := flag.String("engine", "", "drpy 引擎文件路径，如 drpy2.min.js")
	rulePath := flag.String("rule", "", "规则文件路径，如 tuxiaobei.js")
	flag.Parse()

	if *enginePath != "" {
		src, err := os.ReadFile(*enginePath)
		if err != nil {
			fmt.Println("读取引擎失败:", err)
		} else if _, err := newVM().RunString(string(src)); err != nil {
			fmt.Println("加载引擎失败 ->", err)
		} else {
			fmt.Println("加载引擎成功")
		}
	}

	if *rulePath == "" {
		return
	}
	src, err := os.ReadFile(*rulePath)
	if err != nil {
		fmt.Println("读取规则失败:", err)
		return
	}
	vm := newVM()
	if _, err := vm.RunString(string(src)); err != nil {
		fmt.Println("加载规则失败 ->", err)
		return
	}
	rule := vm.Get("rule")
	if rule == nil || goja.IsUndefined(rule) || goja.IsNull(rule) {
		fmt.Println("规则文件没有导出 rule")
		return
	}
	obj := rule.ToObject(vm)
	for _, key := range []string{"title", "host", "url", "detailUrl", "searchUrl", "搜索", "一级", "二级"} {
		fmt.Printf("rule[%s] = %v\n", key, obj.Get(key))
	}
	fmt.Println("rule 里是否有可调用的搜索函数: 否（搜索/一级/二级 都是字符串规则模板）")
}
