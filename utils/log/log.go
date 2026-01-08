package log

import (
	"fmt"
)

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

func Printf(format string, v ...any) {
	l := LogInfo{
		format: format,
		method: "Printf",
		v:      append([]any{}, v...),
	}
	go func() {
		logChan <- l
	}()
}

func Println(v ...any) {
	l := LogInfo{
		method: "Println",
		v:      append([]any{}, v...),
	}
	go func() {
		logChan <- l
	}()
}

func Fatal(v ...any) {
	l := LogInfo{
		method: "Fatal",
		v:      append([]any{}, v...),
	}
	go func() {
		logChan <- l
	}()
}

func Fatalf(format string, v ...any) {
	l := LogInfo{
		format: format,
		method: "Fatalf",
		v:      append([]any{}, v...),
	}
	go func() {
		logChan <- l
	}()
}
