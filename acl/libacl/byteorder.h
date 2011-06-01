#include <endian.h>

#if __BYTE_ORDER == __BIG_ENDIAN
# define cpu_to_le16(w16) le16_to_cpu(w16)
# define le16_to_cpu(w16) ((u_int16_t)((u_int16_t)(w16) >> 8) | \
                           (u_int16_t)((u_int16_t)(w16) << 8))
# define cpu_to_le32(w32) le32_to_cpu(w32)
# define le32_to_cpu(w32) ((u_int32_t)( (u_int32_t)(w32) >>24) | \
                           (u_int32_t)(((u_int32_t)(w32) >> 8) & 0xFF00) | \
                           (u_int32_t)(((u_int32_t)(w32) << 8) & 0xFF0000) | \
			   (u_int32_t)( (u_int32_t)(w32) <<24))
#elif __BYTE_ORDER == __LITTLE_ENDIAN
# define cpu_to_le16(w16) ((u_int16_t)(w16))
# define le16_to_cpu(w16) ((u_int16_t)(w16))
# define cpu_to_le32(w32) ((u_int32_t)(w32))
# define le32_to_cpu(w32) ((u_int32_t)(w32))
#else
# error unknown endianess?
#endif

