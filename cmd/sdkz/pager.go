package main

import (
	"fmt"
	"io"
)

// outputLines 将多行内容全量输出到 out。
//
// 直接顺序输出所有行，不做交互式分页：列表内容本身不长，终端可自然滚动，
// 且避免 raw 终端模式下逐字符读取导致的换行错位。需要翻页时可自行管道到 less。
func outputLines(out io.Writer, lines []string) {
	for _, l := range lines {
		fmt.Fprintln(out, l)
	}
}
