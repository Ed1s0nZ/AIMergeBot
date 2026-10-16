#define _GNU_SOURCE
#define _DARWIN_C_SOURCE
#include <errno.h>
#include <fcntl.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/resource.h>
#include <sys/stat.h>
#include <unistd.h>

static const char *boolean(int yes) { return yes ? "true" : "false"; }
static int limit(int kind, rlim_t soft, rlim_t hard) {
    struct rlimit value = {soft, hard};
    return setrlimit(kind, &value) == 0;
}
static int memory(void) {
    void *baseline = mmap(NULL, 512ULL << 20, PROT_READ | PROT_WRITE,
                          MAP_PRIVATE | MAP_ANONYMOUS, -1, 0);
    int baseline_allowed = baseline != MAP_FAILED;
    if (baseline_allowed) munmap(baseline, 512ULL << 20);
#ifdef RLIMIT_AS
    int set = limit(RLIMIT_AS, 128ULL << 20, 128ULL << 20);
    if (!set) {
        const char *kind = errno == EINVAL ? "invalid_argument" : errno == EPERM ? "permission_denied" : "other";
        printf("{\"set\":false,\"set_error\":\"%s\",\"baseline_mapping_allowed\":%s,\"mmap_blocked\":false}\n", kind, boolean(baseline_allowed));
        return 0;
    }
    errno = 0;
    void *region = mmap(NULL, 512ULL << 20, PROT_READ | PROT_WRITE,
                        MAP_PRIVATE | MAP_ANONYMOUS, -1, 0);
    int blocked = region == MAP_FAILED && errno == ENOMEM;
    if (region != MAP_FAILED) munmap(region, 512ULL << 20);
    printf("{\"set\":true,\"baseline_mapping_allowed\":%s,\"mmap_blocked\":%s,\"pages_touched\":false}\n", boolean(baseline_allowed), boolean(blocked));
#else
    printf("{\"set\":false,\"baseline_mapping_allowed\":%s,\"mmap_blocked\":false}\n", boolean(baseline_allowed));
#endif
    return 0;
}
static int files(const char *root, int aggregate) {
    int set = limit(RLIMIT_FSIZE, 65536, 65536);
    signal(SIGXFSZ, SIG_IGN);
    if (!set) { puts("{\"set\":false}"); return 0; }
    char data[131072]; memset(data, 'x', sizeof(data));
    off_t total = 0;
    int blocked = 0;
    for (int n = 0; n < (aggregate ? 2 : 1); n++) {
        char path[4096];
        int size = snprintf(path, sizeof(path), "%s/limit-%ld-%d", root, (long)getpid(), n);
        if (size < 0 || (size_t)size >= sizeof(path)) return 2;
        int fd = open(path, O_CREAT | O_EXCL | O_WRONLY, 0600);
        if (fd < 0) return 2;
        ssize_t written = write(fd, data, aggregate ? 65536 : sizeof(data));
        if (written < 0 && errno != EFBIG) { close(fd); unlink(path); return 2; }
        if (!aggregate) {
            errno = 0;
            blocked = write(fd, data, 1) == -1 && errno == EFBIG;
        }
        struct stat info;
        if (fstat(fd, &info) != 0 || info.st_size > 65536) { close(fd); unlink(path); return 2; }
        total += info.st_size;
        close(fd); unlink(path);
    }
    printf("{\"set\":true,\"bytes\":%lld,\"blocked\":%s,\"aggregate_exceeds_per_file_limit\":%s}\n",
           (long long)total, boolean(blocked), boolean(total > 65536));
    return 0;
}
static int descriptors(void) {
    int set = limit(RLIMIT_NOFILE, 64, 64);
    if (!set) { puts("{\"set\":false}"); return 0; }
    int fds[128], count = 0, exhausted = 0;
    for (; count < 128; count++) {
        fds[count] = open("/dev/null", O_RDONLY);
        if (fds[count] < 0) { exhausted = errno == EMFILE; break; }
    }
    for (int n = 0; n < count; n++) close(fds[n]);
    printf("{\"set\":true,\"exhausted\":%s,\"opened\":%d}\n", boolean(exhausted), count);
    return 0;
}
int main(int argc, char **argv) {
    limit(RLIMIT_CORE, 0, 0);
    if (argc != 3) return 2;
    if (strcmp(argv[1], "memory") == 0) return memory();
    if (strcmp(argv[1], "file") == 0) return files(argv[2], 0);
    if (strcmp(argv[1], "aggregate") == 0) return files(argv[2], 1);
    if (strcmp(argv[1], "nofile") == 0) return descriptors();
    if (strcmp(argv[1], "inherit") == 0) {
        if (!limit(RLIMIT_NOFILE, 64, 64)) { puts("{\"set\":false}"); return 0; }
        execl(argv[0], argv[0], "inherited", argv[2], (char *)NULL);
        return 2;
    }
    if (strcmp(argv[1], "inherited") == 0) {
        struct rlimit value;
        if (getrlimit(RLIMIT_NOFILE, &value) != 0) return 2;
        errno = 0;
        int denied = !limit(RLIMIT_NOFILE, 128, 128) && errno == EPERM;
        printf("{\"set\":true,\"inherited\":%s,\"raise_denied\":%s,\"unprivileged\":%s}\n",
               boolean(value.rlim_cur == 64 && value.rlim_max == 64), boolean(denied), boolean(geteuid() != 0));
        return 0;
    }
    if (strcmp(argv[1], "cpu") == 0 || strcmp(argv[1], "cpu-hard") == 0) {
        if (!limit(RLIMIT_CPU, 1, 2)) { puts("{\"set\":false}"); return 0; }
        signal(SIGXCPU, strcmp(argv[1], "cpu-hard") == 0 ? SIG_IGN : SIG_DFL);
        puts("{\"set\":true}"); fflush(stdout);
        volatile unsigned long long work = 0;
        for (;;) work++;
    }
    return 2;
}
