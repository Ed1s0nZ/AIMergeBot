FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends gcc libc6-dev git util-linux && rm -rf /var/lib/apt/lists/*
COPY limits.c /tmp/limits.c
RUN cc -std=c11 -O2 -Wall -Wextra -Werror /tmp/limits.c -o /usr/local/bin/limits-probe && rm /tmp/limits.c
COPY git-limits.sh /usr/local/bin/git-limits-probe
RUN chmod 0555 /usr/local/bin/git-limits-probe
ENTRYPOINT ["/usr/local/bin/limits-probe"]
