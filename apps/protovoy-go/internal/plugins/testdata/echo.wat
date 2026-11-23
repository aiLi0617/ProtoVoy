(component
  (core module $m
    (memory (export "memory") 1)
    (global $bump (mut i32) (i32.const 1024))
    (func (export "realloc") (param i32 i32 i32 i32) (result i32)
      (local $p i32)
      global.get $bump local.set $p
      global.get $bump local.get 3 i32.add global.set $bump local.get $p)
    (func (export "echo") (param i32 i32) (result i32)
      i32.const 0 local.get 0 i32.store
      i32.const 4 local.get 1 i32.store i32.const 0))
  (core instance $i (instantiate $m))
  (func (export "transform") (param "input" string) (result string)
    (canon lift (core func $i "echo") (memory $i "memory") (realloc (func $i "realloc")))))
