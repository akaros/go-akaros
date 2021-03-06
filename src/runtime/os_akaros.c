// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "runtime.h"
#include "defs_GOOS_GOARCH.h"
#include "os_GOOS.h"
#include "signal_unix.h"
#include "../cmd/ld/textflag.h"

void runtime·setldt(void)
{
	// Do nothing for now
}

#pragma textflag NOSPLIT
void
runtime·futexsleep(uint32 *addr, uint32 val, int64 ns)
{
	Timespec ts;
	
	if(ns < 0) {
		runtime·futex(addr, FUTEX_WAIT, val, nil, nil, 0);
		return;
	}
	// NOTE: tv_nsec is int64 on amd64, so this assumes a little-endian system.
	ts.tv_nsec = 0;
	ts.tv_sec = runtime·timediv(ns, 1000000000LL, (int32*)&ts.tv_nsec);
	runtime·futex(addr, FUTEX_WAIT, val, &ts, nil, 0);
}

#pragma textflag NOSPLIT
void
runtime·futexwakeup(uint32 *addr, uint32 cnt)
{
	int32 ret = runtime·futex(addr, FUTEX_WAKE, cnt, nil, nil, 0);
	if(ret >= 0)
		return;

	runtime·printf("futexwakeup addr=%p returned %d\n", addr, ret);
	runtime·throw("runtime.futexwakeup");
}

void
runtime·newosproc(M *mp, void *stk)
{
	// Unimplemented for now...
	USED(mp, stk);
	runtime·printf("runtime: failed to create new OS thread (have %d already)\n",
	               runtime·mcount());
	runtime·throw("runtime.newosproc");
}

void
runtime·osinit(void)
{
	runtime·ncpu = MAX(__procinfo.max_vcores, 1);
}

#pragma textflag NOSPLIT
void
runtime·get_random_data(byte **rnd, int32 *rnd_len)
{
	// TODO: revisit and do something similar to Linux with #c/random
	*rnd = nil;
	*rnd_len = 0;
}

void
runtime·goenvs(void)
{
	runtime·goenvs_unix();
}

// Called to initialize a new m (including the bootstrap m).
// Called on the parent thread (main thread in case of bootstrap), can allocate memory.
void
runtime·mpreinit(M *mp)
{
	mp->gsignal = runtime·malg(32*1024);	// OS X wants >=8K, Akaros >=2K
        mp->gsignal->m = mp;
}

// Called to initialize a new m (including the bootstrap m).
// Called on the new thread, can not allocate memory.
void
runtime·minit(void)
{
	// Initialize signal handling.
	runtime·unblocksignals();
        runtime·signalstack((byte*)g->m->gsignal->stack.lo, 32*1024);
}

// Called from dropm to undo the effect of an minit.
void
runtime·unminit(void)
{
        runtime·signalstack(nil, 0);
}

uintptr
runtime·memlimit(void)
{
	// Do nothing for now
	return 0;
}

/*
 * This assembler routine takes the args from registers, puts them on the stack,
 * and calls sighandler().
 */
#pragma cgo_import_static gcc_sigaction
typedef void (*gcc_call_t)(void *arg);
extern gcc_call_t gcc_sigaction;
extern void runtime·sigtramp(void);
extern SigTab runtime·sigtab[];
static Sigset sigset_none;
static Sigset sigset_all = { ~(uint32)0 };

void
runtime·setsig(int32 i, GoSighandler *fn, bool restart)
{
	USED(restart); // Akaros currently only supports the SA_SIGINFO flag
	SigactionT sa;
	runtime·memclr((byte*)&sa, sizeof sa);

	sa.sa_flags = SA_SIGINFO;
	if(fn == runtime·sighandler)
		fn = (void*)runtime·sigtramp;
	sa.sa_sigact = fn;

	SigactionArg sarg;
	sarg.sig = i;
	sarg.act = &sa;
	sarg.oact = nil;
	runtime·asmcgocall(gcc_sigaction, &sarg);
	if (sarg.ret)
		runtime·throw("sigaction failure");
}

GoSighandler*
runtime·getsig(int32 i)
{
	SigactionT sa;
	runtime·memclr((byte*)&sa, sizeof sa);

	SigactionArg sarg;
	sarg.sig = i;
	sarg.act = nil;
	sarg.oact = &sa;
	runtime·asmcgocall(gcc_sigaction, &sarg);
	if (sarg.ret)
		runtime·throw("rt_sigaction read failure");

	if((void*)sa.sa_sigact == runtime·sigtramp)
		return runtime·sighandler;
	return (void*)sa.sa_sigact;
}

#pragma cgo_import_static gcc_sigaltstack
extern gcc_call_t gcc_sigaltstack;
#pragma textflag NOSPLIT
void
runtime·signalstack(byte *p, int32 n)
{
        StackT st;

        st.ss_sp = (void*)p;
        st.ss_size = n;
        st.ss_flags = 0;
        if(p == nil)
                st.ss_flags = SS_DISABLE;
        runtime·asmcgocall(gcc_sigaltstack, &st);
}


void
runtime·unblocksignals(void)
{
	runtime·sigprocmask(SIG_SETMASK, &sigset_none, nil);
}

#pragma textflag NOSPLIT
int8*
runtime·signame(int32 sig)
{
        return runtime·sigtab[sig].name;
}

