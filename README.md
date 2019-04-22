# Orange Applications Runner
**OAR** allows to run programs and measure time and memory they use under Linux.

```
Command line format:
  ./oar [<options>] <program> [<parameters>]
Where options are:
  -h               - show this help
  -t <time-limit>  - time limit, terminate after <time-limit> seconds, you can
                     add "ms" (without quotes) after the number to specify
                     time limit in milliseconds
  -m <mem-limit>   - memory limit, terminate if working set of the process
                     exceeds <mem-limit> bytes, you can add K or M to specify
                     memory limit in kilo- or megabytes
  -r <req-load>    - required load of the processor for this process
                     not to be considered idle. You can add % sign to specify
                     required load in percent, default is 0.05 = 5%
  -y <idle-limit>  - idleness limit, terminate process if it did not load
                     processor for at least <req-load> for <idleness-limit>
  -d <directory>   - make <directory> home directory for process
  -l <login-name>  - create process under <login-name>
  -p <password>    - logins user using <password>
  -i <file>        - redirects standard input stream to the <file>
  -o <file>        - redirects standard output stream to the <file>
  -e <file>        - redirects standard error stream to the <file>
  -x               - return exit code of the application
  -q               - do not display any information on the screen
  -w               - display program window on the screen
  -a               - list of CPUs available to the process (divided by comma).
                     If not specified, child process will be use all available cores.
                     Specify \"-1\" to use single most unload CPU core
  -s <file>        - store statistics in then <file>
  -D var=value     - sets value of the environment variable, current environment
                     is completely ignored in this case
Extended options:
  -Xacp, --allow-create-processes
                   - allow the spawned process to create new processes
  -Xamt, --allow-multi-threaded
                   - allow the spawned process to clone himself for new thread creation,
                     relevanted only if -Xacp is not stated.
  -Xtfce, --terminate-on-first-chance-exceptions
                   - do not ignore exceptions if they are marked as first-chance,
                     required for some old compilers as Borland Delphi
Examples:
  ./oar -t 10s -m 32M a.exe
  ./oar -i input.txt -o output.txt -e error.txt <progname>
Enable debug:
  ./oar -D ~DEBUG <progname>
```

Heavily inspired by [ns-process](https://github.com/teddyking/ns-process)

[Orange eJudje system](http://orange.spbgut.ru)
