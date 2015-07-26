/*
 * getpwent.c -- simulate getpwent() and setpwent()
 */

#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <sys/types.h>
#include <pwd.h>

static struct passwd entries[] = {
	{ .pw_name = (char *) "root", .pw_passwd = (char *) "x",
	  .pw_uid = 0, .pw_gid = 0, .pw_dir = (char *) "/",
	  .pw_shell = (char *) "/bin/sh" },
	{ .pw_name = (char *) "fsgqa", .pw_passwd = (char *) "x",
	  .pw_uid = 31415, .pw_gid = 31415, .pw_dir = (char *) "/home/fsgqa",
	  .pw_shell = (char *) "/bin/sh" },
	{ .pw_name = NULL, .pw_passwd = NULL, .pw_uid = 0, .pw_gid = 0,
	  .pw_dir = NULL, .pw_shell = NULL },
};

static struct passwd *current_pw;

struct passwd *getpwent(void)
{
	if (current_pw == NULL)
		current_pw = entries;
	if (current_pw->pw_name == NULL) {
		return NULL;
	}
	return current_pw++;
}

int setpwent(void)
{
	current_pw = NULL;
}

#if 0
void endpwent(void)
{
	current_pw = NULL;
}
#endif

#ifdef DEBUG
struct passwd *getpwnam(const char *name)
{
	struct passwd *cur;
	
	setpwent();
	while ((cur = getpwent()) != NULL) {
		if (strcmp(name, cur->pw_name) == 0)
			return cur;
	}
	return NULL;
}

struct passwd *getpwuid(uid_t uid)
{
	struct passwd *cur;

	setpwent();
	while ((cur = getpwent()) != NULL) {
		if (uid == cur->pw_uid)
			return cur;
	}
	return NULL;
}

static void print_passwd(struct passwd *pw)
{
	if (pw == NULL)
		printf("NULL entry\n");
	else
		printf("%s:%s:%d:%d:%s:%s:%s\n", pw->pw_name, pw->pw_passwd,
		       pw->pw_uid, pw->pw_gid, pw->pw_name, pw->pw_dir,
		       pw->pw_shell);
}

int main(int argc, char **argv)
{
	struct passwd *pw;
	int err;

	pw = getpwnam("root");
	print_passwd(pw);
	if (!pw)
		err++;
	pw = getpwnam("fsgqa");
	print_passwd(pw);
	if (!pw)
		err++;
	pw = getpwnam("foo");
	print_passwd(pw);
	if (pw)
		err++;
	pw = getpwuid(0);
	print_passwd(pw);
	if (!pw)
		err++;
	pw = getpwuid(31415);
	print_passwd(pw);
	if (!pw)
		err++;
	pw = getpwuid(42);
	print_passwd(pw);
	if (pw)
		err++;
	if (err)
		printf("Failed!\n");
	else
		printf("Succeeded!\n");
	return err;
}
#endif
			
