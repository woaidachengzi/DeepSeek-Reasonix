/* Private macOS acceptance helper. Emit PID\0kernel-cwd\0 without display
 * escaping. Only same-user processes and a bounded explicit PID list. */
#include <errno.h>
#include <limits.h>
#include <libproc.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/proc_info.h>
#include <unistd.h>

int main(int argc, char **argv) {
    if (argc < 2 || argc > 65) return 2;
    for (int index = 1; index < argc; ++index) {
        char *end = NULL;
        errno = 0;
        long number = strtol(argv[index], &end, 10);
        if (errno || !end || *end || number <= 0 || number > INT_MAX) return 2;
        struct proc_bsdinfo identity = {0};
        if (proc_pidinfo((int)number, PROC_PIDTBSDINFO, 0, &identity, sizeof(identity))
                != sizeof(identity) || identity.pbi_uid != getuid()) continue;
        struct proc_vnodepathinfo info = {0};
        if (proc_pidinfo((int)number, PROC_PIDVNODEPATHINFO, 0, &info, sizeof(info))
                != sizeof(info)) continue;
        size_t size = strnlen(info.pvi_cdir.vip_path, sizeof(info.pvi_cdir.vip_path));
        if (!size || size == sizeof(info.pvi_cdir.vip_path)) return 3;
        printf("%ld", number);
        putchar(0);
        fwrite(info.pvi_cdir.vip_path, 1, size, stdout);
        putchar(0);
    }
    return ferror(stdout) ? 3 : 0;
}
