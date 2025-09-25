package slog

// import (
// 	"log/slog"
//
// 	"github.com/RangelReale/ecapplog-go"
// )
//
// func ECLogger() *slog.Logger {
// 	client := ecapplog.NewClient(
// 		ecapplog.WithAppName("svcinit"),
// 	)
// 	client.Open()
// 	return slog.New(ecapplog.NewSLogHandler(client,
// 		ecapplog.WithSlogHandlerOptions(slog.HandlerOptions{
// 			AddSource:   false,
// 			Level:       LevelTrace,
// 			ReplaceAttr: ReplaceAttr,
// 		}),
// 		ecapplog.WithWithSlogHandlerCustomLevelFn(func(level slog.Level) ecapplog.Priority {
// 			if level == LevelTrace {
// 				return ecapplog.Priority_TRACE
// 			}
// 			return ecapplog.Priority_DEBUG
// 		}),
// 		ecapplog.WithSlogHandlerMessageTemplate(`{{if hasField "step"}}[{{field "step"}}] {{end}}{{if hasField "stage"}}[{{field "stage"}}] {{end}}{{if hasField "task"}}[TASK:{{field "task"}}] {{end}}{{.message}}{{if hasField "error"}} ({{field "error"}}){{end}}`),
// 	))
// }
