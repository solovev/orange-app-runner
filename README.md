# Orange Applications Runner
**OAR** allows to run programs and measure time and memory they use under Linux.

```
Command line format:
  oar [<options>] <program> [<parameters>]
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
  -y <idle-limit>  - ildeness limit, terminate process if it did not load
                     processor for at least <req-load> for <idleness-limit>
  -d <directory>   - make <directory> home directory for process
  -l <login-name>  - create process under <login-name>
  -p <password>    - logins user using <password>
  -i <file>        - redirects standart input stream to the <file>
  -o <file>        - redirects standart output stream to the <file>
  -e <file>        - redirects standart error stream to the <file>
  -x               - return exit code of the application
  -q               - do not display any information on the screen
  -w               - display program window on the screen
  -1               - use single CPU/CPU core
  -s <file>        - store statistics in then <file>
  -D var=value     - sets value of the environment variable, current environment
                     is completly ignored in this case
Exteneded options:
  -Xacp, --allow-create-processes
                   - allow the created process to create new processes
  -Xtfce, --terminate-on-first-chance-exceptions
                   - do not ignore exceptions if they are marked as first-chance,
                     required for some old compilers as Borland Delphi
Examples:
  run -t 10s -m 32M a.exe
  run -i input.txt -o output.txt -e error.txt a.exe
```

[Orange eJudje system](orange.spbgut.ru)
