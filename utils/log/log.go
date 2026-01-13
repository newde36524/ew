package log

import (
	"fmt"
)

var IsShow = true

type LogInfo struct {
	format string
	method string
	v      []any
}

var logChan = make(chan LogInfo, 1000)

func init() {
	go func() {
		for v := range logChan {
			switch v.method {
			case "Printf":
				fmt.Println(fmt.Sprintf(v.format, v.v...))
			case "Println":
				fmt.Println(v.v...)
			case "Fatal":
				fmt.Println(v.v...)
			case "Fatalf":
				fmt.Println(fmt.Sprintf(v.format, v.v...))
			default:
			}
		}
	}()
}

func pushLogInfo(logInfo LogInfo) {
	if !IsShow {
		return
	}
	go func() {
		logChan <- logInfo
	}()
}

func Printf(format string, v ...any) {
	l := LogInfo{
		format: format,
		method: "Printf",
		v:      append([]any{}, v...),
	}
	pushLogInfo(l)
}

func Println(v ...any) {
	l := LogInfo{
		method: "Println",
		v:      append([]any{}, v...),
	}
	pushLogInfo(l)
}

func Fatal(v ...any) {
	l := LogInfo{
		method: "Fatal",
		v:      append([]any{}, v...),
	}
	pushLogInfo(l)
}

func Fatalf(format string, v ...any) {
	l := LogInfo{
		format: format,
		method: "Fatalf",
		v:      append([]any{}, v...),
	}
	pushLogInfo(l)
}
