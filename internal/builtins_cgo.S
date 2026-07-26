//go:build cgo

/*
 * Assembled by the C toolchain in cgo builds. This file plays the role the
 * (now non-cgo-only) empty builtins.s plays elsewhere: it marks the package
 * as containing assembly so the bodyless //go:noescape stub in builtins.go
 * is accepted.
 *
 * A hand-written assembly object does not receive a .note.GNU-stack section
 * automatically. Without it, GNU ld marks the entire output's stack executable
 * (RWE) because of this single input, and glibc then refuses to dlopen the
 * GDExtension: "cannot enable executable stack as shared object requires"
 * (graphics.gd issue #318, same failure as noescape_cgo.S / discussion #310).
 * Emit the note explicitly to keep the stack non-executable.
 *
 * ELF-only: Mach-O (darwin/ios) and PE (windows) have no GNU-stack concept and
 * their assemblers reject the directive, so guard it.
 */
#if defined(__ELF__)
.section .note.GNU-stack,"",@progbits
#endif
