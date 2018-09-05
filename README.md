% NETBIN(1) Version 0.1 | netbin Documentation

NAME
====

**netbin** — Serve programs stdin/stdout over tcp or unix sockets

SYNOPSIS
========

Serve a stdin/stdout over tcp or unix sockets. Similar to 
inetd and nc only simpler.

DESCRIPTION
===========


```
netbin OPTIONS CMD CMDARGS
options:
  -addr string
        address to listen on (default "127.0.0.1:5877")
  -domain string
        domain to listen on, valid domains are 'tcp' 'tcp4' 'tcp6' 'unix'  (default "tcp")
  -max-concurrent uint
        Maximum concurrent connections, 0 to disable (default 20)
  -tcp-keepalive uint
        TCP keepalive in seconds, 0 to disable (default 120)
```

BUGS
====

See GitHub Issues: <https://github.com/andrewchambers/netbin/issues>

AUTHOR
======

Andrew Chambers <andrewchamberss@gmail.com>

SEE ALSO
========

