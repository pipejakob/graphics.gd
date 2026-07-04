/* helper_src.h - source of the "borrowed loader" helper program.
 *
 * A static musl binary cannot dlopen() glibc shared libraries directly, so we
 * run this tiny glibc program in-process (see elf_exec in dlopen.c): it links
 * against the host's glibc via the host's ld.so, hands us the real dlopen/dlsym
 * and a set of parked glibc TLS blocks (TCBs), then parks. The musl side switches
 * the CPU TLS register to one of those TCBs around every foreign call.
 *
 * This program is single-sourced here as the HELPER string for two consumers:
 *   1. dlopen.c writes it to disk and compiles it at runtime (fallback path).
 *   2. gen_helper.sh extracts it (fputs(HELPER,...)) and cross-compiles it with
 *      zig against an old glibc, embedding the resulting ELF (helper_bin_*.c) so
 *      the common glibc-host case needs no compiler at all.
 *
 * Keep this the ONLY definition of the helper program. The `struct tls_pool`
 * layout and the callback-array order in main() are an ABI contract with the
 * matching declarations in dlopen.c; change them together.
 */
#ifndef GRAPHICS_GD_DLOPEN_HELPER_SRC_H
#define GRAPHICS_GD_DLOPEN_HELPER_SRC_H

#define HELPER \
  "#define _GNU_SOURCE\n" \
  "#include <dlfcn.h>\n" \
  "#include <stdio.h>\n" \
  "#include <stdlib.h>\n" \
  "#include <pthread.h>\n" \
  "#include <semaphore.h>\n" \
  "#include <stdint.h>\n" \
  "#include <unistd.h>\n" \
  "\n" \
  "#define TLS_POOL_SIZE 64\n" \
  "\n" \
  "/* TLS pool: array of glibc TLS pointers, one per pool thread. */\n" \
  "struct tls_pool {\n" \
  "  void *tls_ptrs[TLS_POOL_SIZE];      /* glibc TLS pointers */\n" \
  "  sem_t ready;                         /* signaled when all threads ready */\n" \
  "  sem_t shutdown;                      /* signaled to shut down threads */\n" \
  "  int count;                           /* number of threads initialized */\n" \
  "  pthread_mutex_t lock;\n" \
  "} __tls_pool;\n" \
  "\n" \
  "/* Minimum usable thread stack: PTHREAD_STACK_MIN can exceed a hardcoded\n" \
  "   16 KiB on some arches/glibc builds (e.g. large SVE state on aarch64), in\n" \
  "   which case pthread_create() fails with EINVAL. Clamp up to the runtime\n" \
  "   minimum so the pool and on-demand factory always succeed. */\n" \
  "static size_t helper_stack_size(void) {\n" \
  "  size_t ss = 16384;\n" \
  "  long m = sysconf(_SC_THREAD_STACK_MIN);\n" \
  "  if (m > 0 && (size_t)m > ss) ss = (size_t)m;\n" \
  "  return ss;\n" \
  "}\n" \
  "\n" \
  "static void *get_tls(void) {\n" \
  "  void *tls;\n" \
  "#if defined(__x86_64__)\n" \
  "  __asm__ volatile(\"mov %%fs:0, %0\" : \"=r\"(tls));\n" \
  "#elif defined(__aarch64__)\n" \
  "  __asm__ volatile(\"mrs %0, tpidr_el0\" : \"=r\"(tls));\n" \
  "#else\n" \
  "#error \"unsupported architecture\"\n" \
  "#endif\n" \
  "  return tls;\n" \
  "}\n" \
  "\n" \
  "static void *pool_thread(void *arg) {\n" \
  "  int idx = (int)(intptr_t)arg;\n" \
  "  pthread_mutex_lock(&__tls_pool.lock);\n" \
  "  __tls_pool.tls_ptrs[idx] = get_tls();\n" \
  "  __tls_pool.count++;\n" \
  "  pthread_mutex_unlock(&__tls_pool.lock);\n" \
  "  sem_post(&__tls_pool.ready); /* signal this thread recorded its TLS */\n" \
  "  sem_wait(&__tls_pool.shutdown); /* sleep forever */\n" \
  "  return NULL;\n" \
  "}\n" \
  "\n" \
  "/* On-demand glibc TCB factory: spawn a parked glibc thread and return its\n" \
  "   TLS pointer, so the musl side can hand fresh glibc TCBs to threads beyond\n" \
  "   the pre-spawned pool (no fixed cap). The musl side serializes calls, so the\n" \
  "   statics below need no locking. */\n" \
  "static sem_t __tcb_ready;\n" \
  "static void *__tcb_captured;\n" \
  "static void *tcb_thread(void *arg) {\n" \
  "  (void)arg;\n" \
  "  __tcb_captured = get_tls();\n" \
  "  sem_post(&__tcb_ready);\n" \
  "  sem_wait(&__tls_pool.shutdown); /* park forever */\n" \
  "  return NULL;\n" \
  "}\n" \
  "void *glibc_tcb_create(void) {\n" \
  "  pthread_attr_t attr;\n" \
  "  pthread_attr_init(&attr);\n" \
  "  pthread_attr_setstacksize(&attr, helper_stack_size());\n" \
  "  sem_init(&__tcb_ready, 0, 0);\n" \
  "  pthread_t t;\n" \
  "  int rc = pthread_create(&t, &attr, tcb_thread, NULL);\n" \
  "  pthread_attr_destroy(&attr);\n" \
  "  if (rc != 0) return NULL;\n" \
  "  pthread_detach(t);\n" \
  "  sem_wait(&__tcb_ready);\n" \
  "  return __tcb_captured;\n" \
  "}\n" \
  "\n" \
  "int main(int argc, char **argv, char **envp) {\n" \
  "  (void)envp;\n" \
  "  char *ep;\n" \
  "  long addr;\n" \
  "  if (argc != 2) {\n" \
  "    fprintf(stderr, \"%s: not intended to be run directly\\n\", argv[0]);\n" \
  "    return 1;\n" \
  "  }\n" \
  "  addr = strtol(argv[1], &ep, 10);\n" \
  "  if (*ep) {\n" \
  "    fprintf(stderr, \"%s: invalid function address\\n\", argv[0]);\n" \
  "    return 2;\n" \
  "  }\n" \
  "  /* Initialize TLS pool */\n" \
  "  sem_init(&__tls_pool.ready, 0, 0);\n" \
  "  sem_init(&__tls_pool.shutdown, 0, 0);\n" \
  "  pthread_mutex_init(&__tls_pool.lock, NULL);\n" \
  "  __tls_pool.count = 0;\n" \
  "  /* Slot 0 is for main thread */\n" \
  "  __tls_pool.tls_ptrs[0] = get_tls();\n" \
  "  __tls_pool.count = 1;\n" \
  "  /* Create pool threads */\n" \
  "  pthread_attr_t attr;\n" \
  "  pthread_attr_init(&attr);\n" \
  "  pthread_attr_setstacksize(&attr, helper_stack_size());\n" \
  "  int created = 0;\n" \
  "  for (int i = 1; i < TLS_POOL_SIZE; i++) {\n" \
  "    pthread_t t;\n" \
  "    if (pthread_create(&t, &attr, pool_thread, (void*)(intptr_t)i) == 0) {\n" \
  "      pthread_detach(t);\n" \
  "      created++;\n" \
  "    }\n" \
  "  }\n" \
  "  pthread_attr_destroy(&attr);\n" \
  "  /* Wait only for the threads that were actually created, so a failed\n" \
  "     pthread_create (e.g. RLIMIT_NPROC in a container) cannot hang us\n" \
  "     forever. Unfilled slots keep NULL TLS pointers; the musl side's\n" \
  "     get_thread_foreign_tls() aborts cleanly if one is ever assigned. */\n" \
  "  for (int i = 0; i < created; i++) sem_wait(&__tls_pool.ready);\n" \
  "  if (created < TLS_POOL_SIZE - 1)\n" \
  "    fprintf(stderr, \"dlopen helper: only %d of %d TLS pool threads \"\n" \
  "                    \"created; foreign calls beyond that use on-demand TCBs\\n\",\n" \
  "            created, TLS_POOL_SIZE - 1);\n" \
  "  return ((int (*)(void *))addr)((void *[]){\n" \
  "      dlopen,\n" \
  "      dlsym,\n" \
  "      dlclose,\n" \
  "      dlerror,\n" \
  "      &__tls_pool,\n" \
  "      glibc_tcb_create,\n" \
  "  });\n" \
  "}\n"

#endif /* GRAPHICS_GD_DLOPEN_HELPER_SRC_H */
