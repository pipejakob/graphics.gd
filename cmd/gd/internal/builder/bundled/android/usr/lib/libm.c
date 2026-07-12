// Stub for android's libm: exists only so the linker can satisfy `-lm`
// when cross-compiling with -nostdlib. No symbols are defined on purpose;
// math symbols stay undefined in the output library and the dynamic
// linker resolves them on-device (libm.so is loaded in every app
// process), the same way libc symbols already resolve.
