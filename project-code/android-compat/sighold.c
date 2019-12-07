/*
 * sighold.c -- emulate sighold() and sigrelse() since bionic doesn't
 * supply them
 */

#include <signal.h>

static void set_to_int(sigset_t *set, int sig)
{
	sigemptyset(set);
	sigaddset(set, sig);
}

int sighold(int sig)
{
	sigset_t set;

	set_to_int(&set, sig);
	return sigprocmask(SIG_BLOCK, &set, NULL);
}


int sigrelse(int sig)
{
	sigset_t set;

	set_to_int(&set, sig);
	return sigprocmask(SIG_UNBLOCK, &set, NULL);
}
