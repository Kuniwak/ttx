package version

// Version はリリースビルド時に goreleaser の ldflags（-X）で上書きされます。
// 通常ビルド（go build / go run）では既定値のままです。const ではなく var なのは
// -X による上書きが var にしか効かないためです。
var Version = "0.0.0"
