// Diagnostic helper: query AMD ADLX for the current FPS metric.
// This is intentionally a separate process so an unavailable driver/API can
// never take down the main UI or the HUD child.
#include <stdio.h>
#include <windows.h>
#include "ADLXHelper/Windows/C/ADLXHelper.h"
#include "IPerformanceMonitoring.h"

int main() {
    ADLX_RESULT r = ADLXHelper_Initialize();
    if (r != ADLX_OK) {
        printf("init=%d\n", (int)r);
        return 2;
    }
    IADLXSystem* sys = ADLXHelper_GetSystemServices();
    if (!sys) {
        printf("no-system\n");
        ADLXHelper_Terminate();
        return 3;
    }
    IADLXPerformanceMonitoringServices* pms = NULL;
    r = sys->pVtbl->GetPerformanceMonitoringServices(sys, &pms);
    if (r != ADLX_OK || !pms) {
        printf("perf=%d\n", (int)r);
        ADLXHelper_Terminate();
        return 4;
    }
    IADLXFPS* fps = NULL;
    r = pms->pVtbl->GetCurrentFPS(pms, &fps);
    if (r != ADLX_OK || !fps) {
        printf("fps=%d\n", (int)r);
        ADLXHelper_Terminate();
        return 5;
    }
    adlx_int value = -1;
    r = fps->pVtbl->FPS(fps, &value);
    printf("fps=%d result=%d\n", (int)value, (int)r);
    fps->pVtbl->Release(fps);
    pms->pVtbl->Release(pms);
    ADLXHelper_Terminate();
    return r == ADLX_OK ? 0 : 6;
}
