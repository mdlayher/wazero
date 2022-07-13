package experimental_test

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/wasi_snapshot_preview1"
)

// listenerWasm was generated by the following:
//	cd testdata; wat2wasm --debug-names listener.wat
//go:embed testdata/listener.wasm
var listenerWasm []byte

// compile-time check to ensure loggerFactory implements experimental.FunctionListenerFactory.
var _ experimental.FunctionListenerFactory = &loggerFactory{}

// loggerFactory implements experimental.FunctionListenerFactory to log all function calls to the console.
type loggerFactory struct{}

// NewListener implements the same method as documented on experimental.FunctionListener.
func (f *loggerFactory) NewListener(moduleName string, fnd api.FunctionDefinition) experimental.FunctionListener {
	return &logger{funcName: []byte(moduleName + "." + funcName(fnd))}
}

// nestLevelKey holds state between logger.Before and logger.After to ensure call depth is reflected.
type nestLevelKey struct{}

// logger implements experimental.FunctionListener to log entrance and exit of each function call.
type logger struct{ funcName []byte }

// Before logs to stdout the module and function name, prefixed with '>>' and indented based on the call nesting level.
func (l *logger) Before(ctx context.Context, _ []uint64) context.Context {
	nestLevel, _ := ctx.Value(nestLevelKey{}).(int)

	l.writeIndented(os.Stdout, true, nestLevel+1)

	// Increase the next nesting level.
	return context.WithValue(ctx, nestLevelKey{}, nestLevel+1)
}

// After logs to stdout the module and function name, prefixed with '<<' and indented based on the call nesting level.
func (l *logger) After(ctx context.Context, _ error, _ []uint64) {
	// Note: We use the nest level directly even though it is the "next" nesting level.
	// This works because our indent of zero nesting is one tab.
	l.writeIndented(os.Stdout, false, ctx.Value(nestLevelKey{}).(int))
}

// funcName returns the name in priority order: first export name, module-defined name, numeric index.
func funcName(fnd api.FunctionDefinition) string {
	if len(fnd.ExportNames()) > 0 {
		return fnd.ExportNames()[0]
	}
	if fnd.Name() != "" {
		return fnd.Name()
	}
	return fmt.Sprintf("[%d]", fnd.Index())
}

// This is a very basic integration of listener. The main goal is to show how it is configured.
func Example_listener() {
	// Set context to one that has an experimental listener
	ctx := context.WithValue(context.Background(), experimental.FunctionListenerFactoryKey{}, &loggerFactory{})

	r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter())
	defer r.Close(ctx) // This closes everything this Runtime created.

	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r); err != nil {
		log.Panicln(err)
	}

	// Compile the WebAssembly module using the default configuration.
	code, err := r.CompileModule(ctx, listenerWasm, wazero.NewCompileConfig())
	if err != nil {
		log.Panicln(err)
	}

	mod, err := r.InstantiateModule(ctx, code, wazero.NewModuleConfig().WithStdout(os.Stdout))
	if err != nil {
		log.Panicln(err)
	}

	_, err = mod.ExportedFunction("rand").Call(ctx, 4)
	if err != nil {
		log.Panicln(err)
	}

	// We should see the same function called twice: directly and indirectly.

	// Output:
	//>>	listener.rand
	//>>		wasi_snapshot_preview1.random_get
	//<<		wasi_snapshot_preview1.random_get
	//>>		wasi_snapshot_preview1.random_get
	//<<		wasi_snapshot_preview1.random_get
	//<<	listener.rand
}

// writeIndented writes an indented message like this: ">>\t\t\t$indentLevel$funcName\n"
func (l *logger) writeIndented(writer io.Writer, before bool, indentLevel int) {
	var message = make([]byte, 0, 2+indentLevel+len(l.funcName)+1)
	if before {
		message = append(message, '>', '>')
	} else { // after
		message = append(message, '<', '<')
	}

	for i := 0; i < indentLevel; i++ {
		message = append(message, '\t')
	}
	message = append(message, l.funcName...)
	message = append(message, '\n')
	_, _ = writer.Write(message)
}
