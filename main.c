#include <ctype.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

static struct {
    char *command;

    int timeLimit, memoryLimit, idleLimit;
    float reqLoad;
    char *directory;
    char *user, *password;

    FILE *input, *output, *error, *store;
    _Bool exitCode, quite, displayWnd;

    char **env;
} cfg;

void parseArgs(int argc, char *argv[], char **envp);
int parseTime(char *value);

void printHelpMessage();
void printError(char *message, ...);

int main(int argc, char *argv[], char **envp) {
    parseArgs(argc, argv, envp);
    printf("%d\n", parseTime("1000ms"));
    //printHelpMessage();
    return EXIT_SUCCESS;
}

void parseArgs(int argc, char *argv[], char **envp) {
    cfg.command = argv[0];
    cfg.timeLimit = cfg.memoryLimit = cfg.idleLimit = -1;
    cfg.env = envp;
    if (strchr(cfg.command, '/') != NULL)
        cfg.command = strrchr(cfg.command, '/') + 1;

    int opt;
    while ((opt = getopt(argc, argv, "h")) != -1) {
        switch (opt) {
            case 'h':
                printHelpMessage();
            case 't':
                cfg.timeLimit = parseTime(optarg);
                break;
            case '?':
                printError("Missing required argument(s), type \"-h\" for details.\n");
            default:
                printError("Unknown option \"%c\", type \"-h\" for details.\n", opt);
        }
    }
}

int parseTime(char *value) {
    int c, result = 0;
    while ((c = *value++) != '\0') {
        switch (c) {
            case 'm':
                printf("%c", *value);
                if (*(value) == 's' && *(value + 1) == '\0')
                    return result * 1000;
                return -1;
            case 's':
                printf("B");
                if (*(value + 1) == '\0')
                    return result;
                return -1;
            default:
                if (isdigit(c))
                    result = result * 10 + (c - '0');
                else
                    return -1;
                break;
        }
    }
    return -1;
}

void printHelpMessage() {
    printf("Command line format: \n");
    printf(" %s [<options] <application> [<parameters>] \n", cfg.command);
    printf("List of options: \n");
    printf(" %s \t \t - %s", "-h", "Print this help message. \n");

    printf(" %s \t - %s", "-t <limit>", "Time limit, terminate after <limit> seconds, you can \n");
    printf("\t \t   add \"ms\" (without quotes) after the number to specify \n");
    printf("\t \t   time limit in milliseconds. \n");

    printf(" %s \t - %s", "-m <limit>", "Memory limit, terminate if working set of the process \n");
    printf("\t \t   exceeds <limit> bytes, you can add K or M to specify \n");
    printf("\t \t   memory limit in kilo- or megabytes. \n");

    printf(" %s \t - %s", "-r <load>", "Required load of the processor for this process \n");
    printf("\t \t   not to be considered idle. You can add %% sign to specify \n");
    printf("\t \t   required load in percent, default is 0.05 = 5%%. \n");

    printf(" %s \t - %s", "-y <limit>", "Idleness limit, terminate process if it did not load \n");
    printf("\t \t   processor for at least <load> for <limit>. \n\n");

    printf(" %s \t - %s", "-d <dir>", "Make <dir> home directory for process. \n");
    printf(" %s \t - %s", "-l <name>", "Create process under <name> user. \n");
    printf(" %s \t - %s", "-p <password>", "Specifies password for user. \n\n");

    printf(" %s \t - %s", "-i <path>", "Redirects standard input stream to the <path>. \n");
    printf(" %s \t - %s", "-o <path>", "Redirects standard output stream to the <path>. \n");
    printf(" %s \t - %s", "-e <path>", "Redirects standard error stream to the <path>. \n");
    printf(" %s \t - %s", "-s <path>", "Save statistics to the <path>. \n\n");

    printf(" %s \t \t - %s", "-x", "Return exit code of the application. \n");
    printf(" %s \t \t - %s", "-q", "Do not display any information on the screen. \n");
    printf(" %s \t \t - %s", "-w", "Display program window on the screen. \n");
    printf(" %s \t \t - %s", "-1", "Use single CPU core. \n\n");

    printf(" %s \t - %s", "-D var=value", "Sets value of the environment variable, current environment \n");
    printf("\t \t   is completely ignored in this case. \n\n");

    printf("Extended options: \n");
    printf(" %s \t \t - %s", "-Xacp", "Allow the spawned process to create new processes. \n");
    printf(" %s \t \t - %s", "-Xamt", "Allow the spawned process to clone himself for new thread \n");
    printf("\t \t   creation, relevanted only if -Xacp is not stated. \n");
    exit(EXIT_SUCCESS);
}

void printError(char *message, ...) {
    va_list args;
    va_start(args, message);
    vfprintf(stderr, message, args);
    va_end(args);
    exit(EXIT_FAILURE);
}