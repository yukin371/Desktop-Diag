// Development-only probe: compare Windows SDK ICMP APIs without changing configuration.
#define WIN32_LEAN_AND_MEAN
#include <winsock2.h>
#include <windows.h>
#include <iphlpapi.h>
#include <icmpapi.h>
#include <sddl.h>
#include <tlhelp32.h>
#include <stdio.h>
#include <string.h>

// print_context compares child-process tokens and loaded security modules across paths.
static void print_context(void) {
    // token is the current process token opened for read-only queries.
    HANDLE token;
    if (!OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &token)) {
        printf("token_error=%lu\n", GetLastError());
        return;
    }
    // buffer stores variable-length token information.
    BYTE buffer[1024];
    // returned receives the token-information byte count.
    DWORD returned;
    // elevation distinguishes ordinary execution from administrator elevation.
    TOKEN_ELEVATION elevation;
    if (GetTokenInformation(token, TokenUser, buffer, sizeof(buffer), &returned)) {
        // sid is the allocated printable form of the token's user identity.
        LPSTR sid = NULL;
        if (ConvertSidToStringSidA(((PTOKEN_USER)buffer)->User.Sid, &sid)) {
            printf("user_sid=%s restricted=%d\n", sid, IsTokenRestricted(token));
            if (LocalFree(sid)) { printf("sid_free_failed\n"); }
        }
    }
    if (GetTokenInformation(token, TokenElevation, &elevation, sizeof(elevation), &returned)) {
        printf("elevated=%lu\n", elevation.TokenIsElevated);
    }
    if (GetTokenInformation(token, TokenIntegrityLevel, buffer, sizeof(buffer), &returned)) {
        // sid identifies the mandatory integrity level, independent of account membership.
        PSID sid = ((PTOKEN_MANDATORY_LABEL)buffer)->Label.Sid;
        printf("integrity_rid=%lu\n", *GetSidSubAuthority(sid, *GetSidSubAuthorityCount(sid) - 1));
    }
    if (!CloseHandle(token)) { printf("token_close_error=%lu\n", GetLastError()); }
    // snapshot enumerates this process's modules without inspecting another process.
    HANDLE snapshot = CreateToolhelp32Snapshot(TH32CS_SNAPMODULE, GetCurrentProcessId());
    if (snapshot == INVALID_HANDLE_VALUE) { return; }
    // module receives one loaded-module entry at a time.
    MODULEENTRY32 module = {0};
    module.dwSize = sizeof(module);
    if (Module32First(snapshot, &module)) {
        do { printf("module=%s\n", module.szModule); } while (Module32Next(snapshot, &module));
    }
    if (!CloseHandle(snapshot)) { printf("module_close_error=%lu\n", GetLastError()); }
}

// run_echo prints the actual API return and last-error code for one controlled request.
static void run_echo(const char *target, const char *label, const char *payload, int explicit_options, int echo2) {
    // handle belongs only to this controlled request.
    HANDLE handle = IcmpCreateFile();
    if (handle == INVALID_HANDLE_VALUE) {
        printf("open_error=%lu\n", GetLastError());
        return;
    }
    // reply has enough capacity for the SDK reply header, payload and error allowance.
    char reply[1024] = {0};
    // options changes only the test packet's TTL, not system configuration.
    IP_OPTION_INFORMATION options = {0};
    options.Ttl = 128;
    // size is bounded by the fixed test payloads in main.
    unsigned short size = (unsigned short)strlen(payload);
    SetLastError(0);
    // count reports successful SDK replies; zero means the reply fields are unavailable.
    DWORD count = echo2
        ? IcmpSendEcho2(handle, NULL, NULL, NULL, inet_addr(target), (LPVOID)payload, size,
                        explicit_options ? &options : NULL, reply, sizeof(reply), 1000)
        : IcmpSendEcho(handle, inet_addr(target), (LPVOID)payload, size,
                       explicit_options ? &options : NULL, reply, sizeof(reply), 1000);
    // error captures the API's last-error code before any other Windows call.
    DWORD error = GetLastError();
    // parsed exposes reply fields, meaningful only when count is nonzero.
    PICMP_ECHO_REPLY parsed = (PICMP_ECHO_REPLY)reply;
    printf("target=%s payload=%s size=%u options=%d api=%s count=%lu error=%lu status=%lu rtt=%lu\n",
           target, label, size, explicit_options, echo2 ? "IcmpSendEcho2" : "IcmpSendEcho",
           count, error, parsed->Status, parsed->RoundTripTime);
    if (!IcmpCloseHandle(handle)) { printf("close_error=%lu\n", GetLastError()); }
}

// main limits probes to loopback and the explicitly supplied gateway.
int main(int argc, char **argv) {
    if (argc != 2) { fprintf(stderr, "usage: icmp-native.exe gateway\n"); return 2; }
    print_context();
    // targets limits the experiment to loopback and the explicitly supplied gateway.
    const char *targets[] = {"127.0.0.1", argv[1]};
    // i visits each target in a fixed order for comparable evidence.
    for (unsigned int i = 0; i < sizeof(targets) / sizeof(targets[0]); i++) {
        run_echo(targets[i], "product", "Desktop-Diag ICMP probe payload!", 0, 0);
        run_echo(targets[i], "ping", "abcdefghijklmnopqrstuvwabcdefghi", 0, 0);
        run_echo(targets[i], "product", "Desktop-Diag ICMP probe payload!", 1, 0);
        run_echo(targets[i], "product", "Desktop-Diag ICMP probe payload!", 0, 1);
    }
    return 0;
}
