/*
 * quota.h -- minimal definitions of kernel quota constants
 */

#define MAXQUOTAS 2
#define USRQUOTA  0		/* element used for user quotas */
#define GRPQUOTA  1		/* element used for group quotas */

/*
 * Command definitions for the 'quotactl' system call.
 * The commands are broken into a main command defined below
 * and a subcommand that is used to convey the type of
 * quota that is being manipulated (see above).
 */
#define SUBCMDMASK  0x00ff
#define SUBCMDSHIFT 8
#define QCMD(cmd, type)  (((cmd) << SUBCMDSHIFT) | ((type) & SUBCMDMASK))

#define Q_SYNC     0x800001	/* sync disk copy of a filesystems quotas */
#define Q_QUOTAON  0x800002	/* turn quotas on */
#define Q_QUOTAOFF 0x800003	/* turn quotas off */
#define Q_GETFMT   0x800004	/* get quota format used on given filesystem */
#define Q_GETINFO  0x800005	/* get information about quota files */
#define Q_SETINFO  0x800006	/* set information about quota files */
#define Q_GETQUOTA 0x800007	/* get user quota structure */
#define Q_SETQUOTA 0x800008	/* set user quota structure */

/* Flags that indicate which fields in dqblk structure are valid.  */
#define QIF_BLIMITS	1
#define QIF_SPACE	2
#define QIF_ILIMITS	4
#define QIF_INODES	8
#define QIF_BTIME	16
#define QIF_ITIME	32
#define QIF_LIMITS	(QIF_BLIMITS | QIF_ILIMITS)
#define QIF_USAGE	(QIF_SPACE | QIF_INODES)
#define QIF_TIMES	(QIF_BTIME | QIF_ITIME)
#define QIF_ALL		(QIF_LIMITS | QIF_USAGE | QIF_TIMES)

struct dqblk
  {
    u_int64_t dqb_bhardlimit;	/* absolute limit on disk quota blocks alloc */
    u_int64_t dqb_bsoftlimit;	/* preferred limit on disk quota blocks */
    u_int64_t dqb_curspace;	/* current quota block count */
    u_int64_t dqb_ihardlimit;	/* maximum # allocated inodes */
    u_int64_t dqb_isoftlimit;	/* preferred inode limit */
    u_int64_t dqb_curinodes;	/* current # allocated inodes */
    u_int64_t dqb_btime;	/* time limit for excessive disk use */
    u_int64_t dqb_itime;	/* time limit for excessive files */
    u_int32_t dqb_valid;	/* bitmask of QIF_* constants */
  };

/*
 * Shorthand notation.
 */
#define	dq_bhardlimit	dq_dqb.dqb_bhardlimit
#define	dq_bsoftlimit	dq_dqb.dqb_bsoftlimit
#define dq_curspace	dq_dqb.dqb_curspace
#define dq_valid	dq_dqb.dqb_valid
#define	dq_ihardlimit	dq_dqb.dqb_ihardlimit
#define	dq_isoftlimit	dq_dqb.dqb_isoftlimit
#define	dq_curinodes	dq_dqb.dqb_curinodes
#define	dq_btime	dq_dqb.dqb_btime
#define	dq_itime	dq_dqb.dqb_itime

#define dqoff(UID)      ((loff_t)((UID) * sizeof (struct dqblk)))


/* Flags that indicate which fields in dqinfo structure are valid.  */
#define IIF_BGRACE	1
#define IIF_IGRACE	2
#define IIF_FLAGS	4
#define IIF_ALL	(IIF_BGRACE | IIF_IGRACE | IIF_FLAGS)

struct dqinfo
  {
    u_int64_t dqi_bgrace;
    u_int64_t dqi_igrace;
    u_int32_t dqi_flags;
    u_int32_t dqi_valid;
  };

extern int quotactl(int __cmd, const char *__special, int __id,
		    caddr_t __addr);
