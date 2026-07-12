// Stub so the linker can satisfy `-lpthread` when cross-compiling with
// -nostdlib. Android has no libpthread.so — pthreads live in libc.so —
// so this stub must stay empty: defining symbols here would record a
// DT_NEEDED on a library that does not exist on-device.
