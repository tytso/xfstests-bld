/*
 * Copyright (c) 1995, 2001-2002 Silicon Graphics, Inc.
 * All Rights Reserved.
 *
 * This program is free software; you can redistribute it and/or
 * modify it under the terms of the GNU General Public License as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it would be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program; if not, write the Free Software Foundation,
 * Inc.,  51 Franklin St, Fifth Floor, Boston, MA  02110-1301  USA
 */

#include <fcntl.h>
#include <stdarg.h>
#include <sys/stat.h>
#include <sys/ioctl.h>
#include <sys/errno.h>
#include <stdint.h>

#include <dmapi.h>
#include <dmapi_kern.h>
#include "dmapi_lib.h"

#define Parg(y)	(void*)va_arg(ap,y)
#define Uarg(y)	(uint64_t)va_arg(ap,y)

static int dmapi_fd = -1;

int
dmi_init_service( char *versionstr )
{
	/* On 2.6 kernels, /dev/dmapi is it */
	dmapi_fd = open( "/dev/dmapi", O_RDWR );
	if (dmapi_fd != -1)
		return 0;

	/* On 2.4 kernels, fs/dmapi_v2 is newer than fs/xfs_dmapi_v2. */
	dmapi_fd = open( "/proc/fs/dmapi_v2", O_RDWR );
	if (dmapi_fd != -1)
		return 0;
	dmapi_fd = open( "/proc/fs/xfs_dmapi_v2", O_RDWR );
	if (dmapi_fd != -1)
		return 0;

	return -1;
}


int
dmi( int opcode, ... )
{
	va_list ap;
	sys_dmapi_args_t kargs;
	sys_dmapi_args_t *u = &kargs;
	int ret = 0;

	if( dmapi_fd == -1 ){
		/* dm_init_service wasn't called, or failed.  The spec
		 * says my behavior is undefined.
		 */
		errno = ENOSYS;
		return -1;
	}

	va_start(ap, opcode);
	switch( opcode ){
/* dm_session */
	case DM_CREATE_SESSION:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(char*);
		DM_Parg(u,3) = Parg(dm_sessid_t*);
		break;
	case DM_QUERY_SESSION:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Uarg(u,2) = Uarg(size_t);
		DM_Parg(u,3) = Parg(void*);
		DM_Parg(u,4) = Parg(size_t*);
		break;
	case DM_GETALL_SESSIONS:
		DM_Uarg(u,1) = Uarg(u_int);
		DM_Parg(u,2) = Parg(dm_sessid_t*);
		DM_Parg(u,3) = Parg(u_int*);
		break;
	case DM_DESTROY_SESSION:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		break;
	case DM_GETALL_TOKENS:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Uarg(u,2) = Uarg(u_int);
		DM_Parg(u,3) = Parg(dm_token_t*);
		DM_Parg(u,4) = Parg(u_int*);
		break;
	case DM_FIND_EVENTMSG:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Uarg(u,2) = Uarg(dm_token_t);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Parg(u,4) = Parg(void*);
		DM_Parg(u,5) = Parg(size_t*);
		break;
/* dm_config */
	case DM_GET_CONFIG:
		DM_Parg(u,1) = Parg(void*);
		DM_Uarg(u,2) = Uarg(size_t);
		DM_Uarg(u,3) = Uarg(dm_config_t);
		DM_Parg(u,4) = Parg(dm_size_t*);
		break;
	case DM_GET_CONFIG_EVENTS:
		DM_Parg(u,1) = Parg(void*);
		DM_Uarg(u,2) = Uarg(size_t);
		DM_Uarg(u,3) = Uarg(u_int);
		DM_Parg(u,4) = Parg(dm_eventset_t*);
		DM_Parg(u,5) = Parg(u_int*);
		break;
/* dm_attr */
	case DM_GET_FILEATTR:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Uarg(u,5) = Uarg(u_int);
		DM_Parg(u,6) = Parg(dm_stat_t*);
		break;
	case DM_SET_FILEATTR:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Uarg(u,5) = Uarg(u_int);
		DM_Parg(u,6) = Parg(dm_fileattr_t*);
		break;
/* dm_bulkattr */
	case DM_INIT_ATTRLOC:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Parg(u,5) = Parg(dm_attrloc_t*);
		break;
	case DM_GET_BULKATTR:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Uarg(u,5) = Uarg(u_int);
		DM_Parg(u,6) = Parg(dm_attrloc_t*);
		DM_Uarg(u,7) = Uarg(size_t);
		DM_Parg(u,8) = Parg(void*);
		DM_Parg(u,9) = Parg(size_t*);
		break;
	case DM_GET_DIRATTRS:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Uarg(u,5) = Uarg(u_int);
		DM_Parg(u,6) = Parg(dm_attrloc_t*);
		DM_Uarg(u,7) = Uarg(size_t);
		DM_Parg(u,8) = Parg(void*);
		DM_Parg(u,9) = Parg(size_t*);
		break;
	case DM_GET_BULKALL:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Uarg(u,5) = Uarg(u_int);
		DM_Parg(u,6) = Parg(dm_attrname_t*);
		DM_Parg(u,7) = Parg(dm_attrloc_t*);
		DM_Uarg(u,8) = Uarg(size_t);
		DM_Parg(u,9) = Parg(void*);
		DM_Parg(u,10) = Parg(size_t*);
		break;
/* dm_dmattr */
	case DM_CLEAR_INHERIT:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Parg(u,5) = Parg(dm_attrname_t*);
		break;
	case DM_GET_DMATTR:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Parg(u,5) = Parg(dm_attrname_t*);
		DM_Uarg(u,6) = Uarg(size_t);
		DM_Parg(u,7) = Parg(void*);
		DM_Parg(u,8) = Parg(size_t*);
		break;
	case DM_GETALL_DMATTR:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Uarg(u,5) = Uarg(size_t);
		DM_Parg(u,6) = Parg(void*);
		DM_Parg(u,7) = Parg(size_t*);
		break;
	case DM_GETALL_INHERIT:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Uarg(u,5) = Uarg(u_int);
		DM_Parg(u,6) = Parg(dm_inherit_t*);
		DM_Parg(u,7) = Parg(u_int*);
		break;
	case DM_REMOVE_DMATTR:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Uarg(u,5) = Uarg(int);
		DM_Parg(u,6) = Parg(dm_attrname_t*);
		break;
	case DM_SET_DMATTR:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Parg(u,5) = Parg(dm_attrname_t*);
		DM_Uarg(u,6) = Uarg(int);
		DM_Uarg(u,7) = Uarg(size_t);
		DM_Parg(u,8) = Parg(void*);
		break;
	case DM_SET_INHERIT:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Parg(u,5) = Parg(dm_attrname_t*);
		DM_Uarg(u,6) = Uarg(mode_t);
		break;
	case DM_SET_RETURN_ON_DESTROY:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Parg(u,5) = Parg(dm_attrname_t*);
		DM_Uarg(u,6) = Uarg(dm_boolean_t);
		break;
/* dm_event */
	case DM_GET_EVENTS:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Uarg(u,2) = Uarg(u_int);
		DM_Uarg(u,3) = Uarg(u_int);
		DM_Uarg(u,4) = Uarg(size_t);
		DM_Parg(u,5) = Parg(void*);
		DM_Parg(u,6) = Parg(size_t*);
		break;
	case DM_RESPOND_EVENT:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Uarg(u,2) = Uarg(dm_token_t);
		DM_Uarg(u,3) = Uarg(dm_response_t);
		DM_Uarg(u,4) = Uarg(int);
		DM_Uarg(u,5) = Uarg(size_t);
		DM_Parg(u,6) = Parg(void*);
		break;
	case DM_GET_EVENTLIST:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Uarg(u,5) = Uarg(u_int);
		DM_Parg(u,6) = Parg(dm_eventset_t*);
		DM_Parg(u,7) = Parg(u_int*);
		break;
	case DM_SET_EVENTLIST:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Parg(u,5) = Parg(dm_eventset_t*);
		DM_Uarg(u,6) = Uarg(u_int);
		break;
	case DM_SET_DISP:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Parg(u,5) = Parg(dm_eventset_t*);
		DM_Uarg(u,6) = Uarg(u_int);
		break;
	case DM_CREATE_USEREVENT:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Uarg(u,2) = Uarg(size_t);
		DM_Parg(u,3) = Parg(void*);
		DM_Parg(u,4) = Parg(dm_token_t*);
		break;
	case DM_SEND_MSG:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Uarg(u,2) = Uarg(dm_msgtype_t);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Parg(u,4) = Parg(void*);
		break;
	case DM_MOVE_EVENT:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Uarg(u,2) = Uarg(dm_token_t);
		DM_Uarg(u,3) = Uarg(dm_sessid_t);
		DM_Parg(u,4) = Parg(dm_token_t*);
		break;
	case DM_PENDING:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Uarg(u,2) = Uarg(dm_token_t);
		DM_Parg(u,3) = Parg(dm_timestruct_t*);
		break;
	case DM_GETALL_DISP:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Uarg(u,2) = Uarg(size_t);
		DM_Parg(u,3) = Parg(void*);
		DM_Parg(u,4) = Parg(size_t*);
		break;
/* dm_handle */
	case DM_PATH_TO_HANDLE:
		DM_Parg(u,1) = Parg(char*);
		DM_Parg(u,2) = Parg(char*);
		DM_Parg(u,3) = Parg(size_t*);
		break;
	case DM_PATH_TO_FSHANDLE:
		DM_Parg(u,1) = Parg(char*);
		DM_Parg(u,2) = Parg(char*);
		DM_Parg(u,3) = Parg(size_t*);
		break;
	case DM_FD_TO_HANDLE:
		DM_Uarg(u,1) = Uarg(int);
		DM_Parg(u,2) = Parg(char*);
		DM_Parg(u,3) = Parg(size_t*);
		break;
	case DM_CREATE_BY_HANDLE:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Parg(u,5) = Parg(void*);
		DM_Uarg(u,6) = Uarg(size_t);
		DM_Parg(u,7) = Parg(char*);
		break;
	case DM_MKDIR_BY_HANDLE:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Parg(u,5) = Parg(void*);
		DM_Uarg(u,6) = Uarg(size_t);
		DM_Parg(u,7) = Parg(char*);
		break;
	case DM_SYMLINK_BY_HANDLE:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Parg(u,5) = Parg(void*);
		DM_Uarg(u,6) = Uarg(size_t);
		DM_Parg(u,7) = Parg(char*);
		DM_Parg(u,8) = Parg(char*);
		break;
/* dm_handle2path */
	case DM_OPEN_BY_HANDLE:
		DM_Uarg(u,1) = Uarg(unsigned int);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(int);
		break;
/* dm_hole */
	case DM_GET_ALLOCINFO:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Parg(u,5) = Parg(dm_off_t*);
		DM_Uarg(u,6) = Uarg(u_int);
		DM_Parg(u,7) = Parg(dm_extent_t*);
		DM_Parg(u,8) = Parg(u_int*);
		break;
	case DM_PROBE_HOLE:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Uarg(u,5) = Uarg(dm_off_t);
		DM_Uarg(u,6) = Uarg(dm_size_t);
		DM_Parg(u,7) = Parg(dm_off_t*);
		DM_Parg(u,8) = Parg(dm_size_t*);
		break;
	case DM_PUNCH_HOLE:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Uarg(u,5) = Uarg(dm_off_t);
		DM_Uarg(u,6) = Uarg(dm_size_t);
		break;
/* dm_mountinfo */
	case DM_GET_MOUNTINFO:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Uarg(u,5) = Uarg(size_t);
		DM_Parg(u,6) = Parg(void*);
		DM_Parg(u,7) = Parg(size_t*);
		break;
/* dm_rdwr */
	case DM_READ_INVIS:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Uarg(u,5) = Uarg(dm_off_t);
		DM_Uarg(u,6) = Uarg(dm_size_t);
		DM_Parg(u,7) = Parg(void*);
		break;
	case DM_WRITE_INVIS:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Uarg(u,5) = Uarg(int);
		DM_Uarg(u,6) = Uarg(dm_off_t);
		DM_Uarg(u,7) = Uarg(dm_size_t);
		DM_Parg(u,8) = Parg(void*);
		break;
	case DM_SYNC_BY_HANDLE:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		break;
	case DM_GET_DIOINFO:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Parg(u,5) = Parg(dm_dioinfo_t*);
		break;
/* dm_region */
	case DM_GET_REGION:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Uarg(u,5) = Uarg(u_int);
		DM_Parg(u,6) = Parg(dm_region_t*);
		DM_Parg(u,7) = Parg(u_int*);
		break;
	case DM_SET_REGION:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Uarg(u,5) = Uarg(u_int);
		DM_Parg(u,6) = Parg(dm_region_t*);
		DM_Parg(u,7) = Parg(dm_boolean_t*);
		break;
/* dm_right */
	case DM_DOWNGRADE_RIGHT:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		break;
	case DM_OBJ_REF_HOLD:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Uarg(u,2) = Uarg(dm_token_t);
		DM_Parg(u,3) = Parg(void*);
		DM_Uarg(u,4) = Uarg(size_t);
		break;
	case DM_OBJ_REF_QUERY:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Uarg(u,2) = Uarg(dm_token_t);
		DM_Parg(u,3) = Parg(void*);
		DM_Uarg(u,4) = Uarg(size_t);
		break;
	case DM_OBJ_REF_RELE:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Uarg(u,2) = Uarg(dm_token_t);
		DM_Parg(u,3) = Parg(void*);
		DM_Uarg(u,4) = Uarg(size_t);
		break;
	case DM_QUERY_RIGHT:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Parg(u,5) = Parg(dm_right_t*);
		break;
	case DM_RELEASE_RIGHT:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		break;
	case DM_REQUEST_RIGHT:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		DM_Uarg(u,5) = Uarg(u_int);
		DM_Uarg(u,6) = Uarg(dm_right_t);
		break;
	case DM_UPGRADE_RIGHT:
		DM_Uarg(u,1) = Uarg(dm_sessid_t);
		DM_Parg(u,2) = Parg(void*);
		DM_Uarg(u,3) = Uarg(size_t);
		DM_Uarg(u,4) = Uarg(dm_token_t);
		break;
	default:
		errno = ENOSYS;
		ret = -1;
		break;
	}
	va_end(ap);

	if( ret != -1 )
		ret = ioctl( dmapi_fd, opcode, &kargs );

	return(ret);
}
