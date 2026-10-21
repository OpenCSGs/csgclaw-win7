#define WIN32_LEAN_AND_MEAN
#define _WIN32_WINNT 0x0601
#include <Windows.h>

/* Optional Windows 8/10 facilities are resolved at runtime.
 * Required classic COM memory management is forwarded to ole32 on Windows 7.
 * No sandbox permission checks or cryptographic routines are replaced here.
 */
typedef LONG (WINAPI *find_packages_fn)(const WCHAR *, UINT32, UINT32 *, WCHAR **, UINT32 *, WCHAR *, UINT32 *);
typedef LONG (WINAPI *package_family_fn)(HANDLE, UINT32 *, WCHAR *);
typedef HRESULT (WINAPI *resize_console_fn)(HANDLE, COORD);
typedef HRESULT (WINAPI *increment_mta_fn)(void **);
typedef HRESULT (WINAPI *activation_factory_fn)(void *, const GUID *, void **);
typedef void (WINAPI *task_mem_free_fn)(void *);
static INIT_ONCE api_once = INIT_ONCE_STATIC_INIT;
static package_family_fn package_family;
static find_packages_fn find_packages;
static resize_console_fn resize_console;
static increment_mta_fn increment_mta;
static activation_factory_fn activation_factory;
static task_mem_free_fn task_mem_free;

static BOOL CALLBACK load_system_apis(PINIT_ONCE once, PVOID parameter, PVOID *context) {
    (void)once; (void)parameter; (void)context;
    HMODULE kernel = GetModuleHandleW(L"kernel32.dll");
    HMODULE ole = LoadLibraryExW(L"ole32.dll", NULL, 0x800);
    HMODULE combase = LoadLibraryExW(L"combase.dll", NULL, 0x800);
    HMODULE winrt = LoadLibraryExW(L"api-ms-win-core-winrt-l1-1-0.dll", NULL, 0x800);
    package_family = (package_family_fn)GetProcAddress(kernel, "GetPackageFamilyName");
    find_packages = (find_packages_fn)GetProcAddress(kernel, "FindPackagesByPackageFamily");
    resize_console = (resize_console_fn)GetProcAddress(kernel, "ResizePseudoConsole");
    if (ole) task_mem_free = (task_mem_free_fn)GetProcAddress(ole, "CoTaskMemFree");
    if (combase) increment_mta = (increment_mta_fn)GetProcAddress(combase, "CoIncrementMTAUsage");
    if (winrt) activation_factory = (activation_factory_fn)GetProcAddress(winrt, "RoGetActivationFactory");
    return TRUE;
}

LONG WINAPI GetPackageFamilyName(HANDLE process, UINT32 *length, WCHAR *family) {
    InitOnceExecuteOnce(&api_once, load_system_apis, NULL, NULL);
    if (package_family) return package_family(process, length, family);
    if (length) *length = 0;
    return 15700; /* APPMODEL_ERROR_NO_PACKAGE: Win7 has no app-package identity. */
}
HRESULT WINAPI ResizePseudoConsole(HANDLE console, COORD size) {
    InitOnceExecuteOnce(&api_once, load_system_apis, NULL, NULL);
    if (resize_console) return resize_console(console, size);
    return (HRESULT)0x80004001L; /* E_NOTIMPL; no fake PTY success. */
}
HRESULT WINAPI CoIncrementMTAUsage(void **cookie) {
    InitOnceExecuteOnce(&api_once, load_system_apis, NULL, NULL);
    if (increment_mta) return increment_mta(cookie);
    if (cookie) *cookie = NULL;
    return (HRESULT)0x80004001L;
}
HRESULT WINAPI RoGetActivationFactory(void *class_name, const GUID *iid, void **factory) {
    InitOnceExecuteOnce(&api_once, load_system_apis, NULL, NULL);
    if (activation_factory) return activation_factory(class_name, iid, factory);
    if (factory) *factory = NULL;
    return (HRESULT)0x80004001L;
}
void WINAPI CoTaskMemFree(void *memory) {
    if (!memory) return;
    InitOnceExecuteOnce(&api_once, load_system_apis, NULL, NULL);
    if (!task_mem_free) {
        /* KB3063858 (or a superseding update) is a required loader baseline. */
        TerminateProcess(GetCurrentProcess(), 0xc0000135UL);
        return;
    }
    task_mem_free(memory);
}

/* Strong import-pointer definitions take precedence over modern import libs. */
package_family_fn __imp_GetPackageFamilyName = GetPackageFamilyName;
resize_console_fn __imp_ResizePseudoConsole = ResizePseudoConsole;
increment_mta_fn __imp_CoIncrementMTAUsage = CoIncrementMTAUsage;
activation_factory_fn __imp_RoGetActivationFactory = RoGetActivationFactory;
task_mem_free_fn __imp_CoTaskMemFree = CoTaskMemFree;

LONG WINAPI FindPackagesByPackageFamily(const WCHAR *family, UINT32 filters, UINT32 *count, WCHAR **names, UINT32 *length, WCHAR *buffer, UINT32 *properties) {
    InitOnceExecuteOnce(&api_once, load_system_apis, NULL, NULL);
    if (find_packages) return find_packages(family, filters, count, names, length, buffer, properties);
    if (!count || !length) return ERROR_INVALID_PARAMETER;
    *count = 0;
    *length = 0;
    return ERROR_SUCCESS; /* Windows 7 has no MSIX package registrations. */
}
find_packages_fn __imp_FindPackagesByPackageFamily = FindPackagesByPackageFamily;
