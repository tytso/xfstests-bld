/*
 * getgrent.c -- simulate getgrent(), setgrent(), endgrent()
 */

#define __USE_XOPEN

#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <sys/types.h>
#include <grp.h>

static struct group entries[] = {
	{ .gr_name = (char *) "root", .gr_passwd = (char *) "x",
	  .gr_gid = 0, .gr_mem = NULL },
	{ .gr_name = (char *) "fsgqa", .gr_passwd = (char *) "x",
	  .gr_gid = 31415, .gr_mem = NULL },
	{ .gr_name = NULL, .gr_passwd = NULL, .gr_gid = 0, .gr_mem = NULL },
};

static struct group *current_grp;

struct group *getgrent(void)
{
	if (current_grp == NULL)
		current_grp = entries;
	if (current_grp->gr_name == NULL) {
		return NULL;
	}
	return current_grp++;
}

void setgrent(void)
{
	current_grp = NULL;
}

void endgrent(void)
{
	current_grp = NULL;
}

#ifdef DEBUG
struct group *getgrnam(const char *name)
{
	struct group *cur;
	
	setgrent();
	while ((cur = getgrent()) != NULL) {
		if (strcmp(name, cur->gr_name) == 0)
			return cur;
	}
	return NULL;
}

struct group *getgrgid(gid_t gid)
{
	struct group *cur;

	setgrent();
	while ((cur = getgrent()) != NULL) {
		if (gid == cur->gr_gid)
			return cur;
	}
	return NULL;
}

static void print_group(struct group *grp)
{
	if (grp == NULL)
		printf("NULL entry\n");
	else
		printf("%s:%s:%d\n", grp->gr_name,
		       grp->gr_passwd, grp->gr_gid);
}

int main(int argc, char **argv)
{
	struct group *grp;
	int err;

	grp = getgrnam("root");
	print_group(grp);
	if (!grp)
		err++;
	grp = getgrnam("fsgqa");
	print_group(grp);
	if (!grp)
		err++;
	grp = getgrnam("foo");
	print_group(grp);
	if (grp)
		err++;
	grp = getgrgid(0);
	print_group(grp);
	if (!grp)
		err++;
	grp = getgrgid(31415);
	print_group(grp);
	if (!grp)
		err++;
	grp = getgrgid(42);
	print_group(grp);
	if (grp)
		err++;
	if (err)
		printf("Failed!\n");
	else
		printf("Succeeded!\n");
	return err;
}
#endif
			
