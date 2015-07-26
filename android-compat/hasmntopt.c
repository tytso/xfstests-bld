/*
 * telldir.c -- Android compatibility hack because bionic doesn't
 * support hasmntopt()
 */

#include <stdio.h>
#include <string.h>
#include <mntent.h>

char *hasmntopt(const struct mntent *mnt, const char *opt)
{
	char	*cp = mnt->mnt_opts;
	int	len = strlen(opt);
	
	if (!mnt->mnt_opts)
		return 0;

	while (1) {
		if (!(cp = strstr(cp, opt)))
			break;
		if ((cp == mnt->mnt_opts || cp[-1]==',') &&
		    (cp[len] == 0 || cp[len] == ',' || cp[len] == '='))
			return cp;
		cp += (len + 1);
	}
	return 0;	
}

#ifdef DEBUG
struct test_list {
	const char *opt;
	int expected;
} tst[] = {
	{ "foo", 1 },
	{ "bar", 1 },
	{ "baz", 1 },
	{ "quux", 0 },
	{ "zork", 0 },
	{ NULL, 0 }
};

int main(int argc, char **argv)
{
	struct mntent m;
	struct test_list *t;
	int errors = 0;

	memset(&m, 0, sizeof(m));
	m.mnt_opts = "foo,bar=quux,baz";

	for (t = tst; t->opt; t++) {
		char *cp;
		int actual;

		cp = hasmntopt(&m, t->opt);
		actual = (cp != NULL);
		printf("hasmntopt(m, %s) returned %s: %s\n",
		       t->opt, cp ? cp : "NULL", actual == t->expected ?
		       "OK" : "FAIL");
		if (actual != t->expected)
			errors++;
	}
	return errors;
}
#endif
