
#include <stdio.h>
#include <stdlib.h>

int lio_listio(int mode, void *aiocb_list[], int nitems, void *sevp)
{
	fprintf(stderr, "lio_listio not implemented!\n");
	abort();
}
