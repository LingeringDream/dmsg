package plugin

import (
"context"
"github.com/tetratelabs/wazero"
"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

type Engine struct {
ctx context.Context
r   wazero.Runtime
}

func Init(ctx context.Context) (*Engine, error) {
	r := wazero.NewRuntime(ctx)

_, err := wasi_snapshot_preview1.Instantiate(ctx, r)
if err != nil {
return nil, err
}
return &Engine{ctx: ctx, r: r}, nil
}

// LoadModule compiles and instantiates a wasm module
func (e *Engine) LoadModule(name string, bin []byte) error {
  code, err := e.r.CompileModule(e.ctx, bin)
  if err != nil {
    return err
  }
  _, err = e.r.InstantiateModule(e.ctx, code, wazero.NewModuleConfig().WithName(name))
  return err
}

func (e *Engine) Close() {
e.r.Close(e.ctx)
}
