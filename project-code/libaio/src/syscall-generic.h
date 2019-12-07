#define _GNU_SOURCE
#include <unistd.h>
#include <sys/syscall.h>

#define io_syscall1(type,fname,sname,type1,arg1)			\
type fname(type1 arg1)							\
{									\
	return (type) syscall(__NR_##sname, arg1);			\
}

#define io_syscall2(type,fname,sname,type1,arg1,type2,arg2)		\
type fname(type1 arg1, type2 arg2)					\
{									\
	return (type) syscall(__NR_##sname, arg1, arg2);		\
}

#define io_syscall3(type,fname,sname,type1,arg1,type2,arg2,type3,arg3)	\
type fname(type1 arg1, type2 arg2, type3 arg3)				\
{									\
	return (type) syscall(__NR_##sname, arg1, arg2, arg3);		\
}

#define io_syscall4(type,fname,sname,type1,arg1,type2,arg2,type3,arg3,type4,arg4) \
type fname (type1 arg1, type2 arg2, type3 arg3, type4 arg4)		\
{									\
	return (type) syscall(__NR_##sname, arg1, arg2, arg3, arg4);	\
}

#define io_syscall5(type,fname,sname,type1,arg1,type2,arg2,type3,arg3,type4,arg4, \
	  type5,arg5)							\
type fname (type1 arg1,type2 arg2,type3 arg3,type4 arg4,type5 arg5)	\
{									\
	return (type) syscall(__NR_##sname, arg1, arg2, arg3, arg4, arg5); \
}
