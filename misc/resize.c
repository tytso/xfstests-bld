/*
 * Originally from resize.c, but significantly simplified to remove
 * unneeded functionality and remove dependencies for kvm-xfstests.
 */

/* $XTermId: resize.c,v 1.135 2015/04/10 09:00:41 tom Exp $ */

/*
 * Copyright 2003-2014,2015 by Thomas E. Dickey
 *
 *                         All Rights Reserved
 *
 * Permission is hereby granted, free of charge, to any person obtaining a
 * copy of this software and associated documentation files (the
 * "Software"), to deal in the Software without restriction, including
 * without limitation the rights to use, copy, modify, merge, publish,
 * distribute, sublicense, and/or sell copies of the Software, and to
 * permit persons to whom the Software is furnished to do so, subject to
 * the following conditions:
 *
 * The above copyright notice and this permission notice shall be included
 * in all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS
 * OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
 * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 * IN NO EVENT SHALL THE ABOVE LISTED COPYRIGHT HOLDER(S) BE LIABLE FOR ANY
 * CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
 * TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
 * SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 *
 * Except as contained in this notice, the name(s) of the above copyright
 * holders shall not be used in advertising or otherwise to promote the
 * sale, use or other dealings in this Software without prior written
 * authorization.
 *
 *
 * Copyright 1987 by Digital Equipment Corporation, Maynard, Massachusetts.
 *
 *                         All Rights Reserved
 *
 * Permission to use, copy, modify, and distribute this software and its
 * documentation for any purpose and without fee is hereby granted,
 * provided that the above copyright notice appear in all copies and that
 * both that copyright notice and this permission notice appear in
 * supporting documentation, and that the name of Digital Equipment
 * Corporation not be used in advertising or publicity pertaining to
 * distribution of the software without specific, written prior permission.
 *
 *
 * DIGITAL DISCLAIMS ALL WARRANTIES WITH REGARD TO THIS SOFTWARE, INCLUDING
 * ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS, IN NO EVENT SHALL
 * DIGITAL BE LIABLE FOR ANY SPECIAL, INDIRECT OR CONSEQUENTIAL DAMAGES OR
 * ANY DAMAGES WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS,
 * WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION,
 * ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS
 * SOFTWARE.
 */

/* resize.c */

#include <stdio.h>
#include <unistd.h>
#include <stdlib.h>
#include <string.h>
#include <termios.h>
#include <signal.h>
#include <sys/ioctl.h>
#include <ctype.h>
#include <errno.h>

#define ESCAPE(string) "\033" string

#define	TIMEOUT		10

static char *myname;
static int tty;
static FILE *ttyfp;
static struct termios tioorig;

static const char *getsize = 
	ESCAPE("7") ESCAPE("[r") ESCAPE("[999;999H") ESCAPE("[6n");
static const char * restore = ESCAPE("8");
static const char * size = ESCAPE("[%d;%dR");

static void
failed(const char *s)
{
    int save = errno;
    write(2, myname, strlen(myname));
    write(2, ": ", (size_t) 2);
    errno = save;
    perror(s);
    exit(1);
}

/* ARGSUSED */
static void
onintr(int sig)
{
    (void) tcsetattr(tty, TCSADRAIN, &tioorig);
    exit(1);
}

static void
resize_timeout(int sig)
{
    fprintf(stderr, "\n%s: Time out occurred\r\n", myname);
    onintr(sig);
}

static void
readstring(FILE *fp, char *buf, const char *str)
{
    int last, c;

    signal(SIGALRM, resize_timeout);
    alarm(TIMEOUT);

    if ((c = getc(fp)) == 0233) {	/* meta-escape, CSI */
	c = ESCAPE("")[0];
	*buf++ = (char) c;
	*buf++ = '[';
    } else {
	*buf++ = (char) c;
    }
    if (c != *str) {
	fprintf(stderr, "%s: unknown character, exiting.\r\n", myname);
	onintr(0);
    }
    last = str[strlen(str) - 1];
    while ((*buf++ = (char) getc(fp)) != last) {
	;
    }
    alarm(0);
    *buf = 0;
}

int
main(int argc, char **argv)
{
    int rc;
    int rows, cols;
    struct termios tio;
    char buf[BUFSIZ];
    struct winsize ts;

    myname = argv[0];

    if ((ttyfp = fopen("/dev/tty", "r+")) == NULL) {
	fprintf(stderr, "%s:  can't open terminal\n", myname);
	exit(1);
    }
    tty = fileno(ttyfp);

    rc = tcgetattr(tty, &tioorig);
    tio = tioorig;
    tio.c_iflag &= ~ICRNL;
    tio.c_lflag &= ~(ICANON | ECHO);
    tio.c_cflag |= CS8;
    tio.c_cc[VMIN] = 6;
    tio.c_cc[VTIME] = 1;

    if (rc != 0)
	failed("get tty settings");

    signal(SIGINT, onintr);
    signal(SIGQUIT, onintr);
    signal(SIGTERM, onintr);

    rc = tcsetattr(tty, TCSADRAIN, &tio);

    write(tty, getsize, strlen(getsize));
    readstring(ttyfp, buf, size);
    if (sscanf(buf, size, &rows, &cols) != 2) {
	fprintf(stderr, "%s: Can't get rows and columns\r\n", myname);
	onintr(0);
    }
    write(tty, restore, strlen(restore));

    rc = tcsetattr(tty, TCSADRAIN, &tioorig);
    if (rc != 0)
	failed("set tty settings");

    signal(SIGINT, SIG_DFL);
    signal(SIGQUIT, SIG_DFL);
    signal(SIGTERM, SIG_DFL);

    ts.ws_xpixel = 0;
    ts.ws_ypixel = 0;
    rc = ioctl(tty, TIOCGWINSZ, &ts);
    if (rc >= 0) {
	    if (ts.ws_col)
		    ts.ws_xpixel = (cols * (ts.ws_xpixel / ts.ws_col));
	    if (ts.ws_row)
		    ts.ws_ypixel = (rows * (ts.ws_ypixel / ts.ws_row));
    }
    ts.ws_row = rows;
    ts.ws_col = cols;

    rc = ioctl(tty, TIOCSWINSZ, &ts);
    if (rc < 0)
	    perror("TIOCWINSZ");

    printf("COLUMNS=%d\nLINES=%d\n",  cols, rows);
    exit(0);
}
