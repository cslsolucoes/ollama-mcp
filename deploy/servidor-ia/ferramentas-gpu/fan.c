#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <nvml.h>
// fan <gpu> <pct|auto>
int main(int c,char**v){ if(c<3)return 2; nvmlInit_v2(); nvmlDevice_t d; if(nvmlDeviceGetHandleByIndex_v2(atoi(v[1]),&d))return 1;
 unsigned n=0; nvmlReturn_t r=nvmlDeviceGetNumFans(d,&n); if(r){printf("GPU %s: numfans %s\n",v[1],nvmlErrorString(r));return 1;}
 for(unsigned f=0;f<n;f++){ r=!strcmp(v[2],"auto")?nvmlDeviceSetDefaultFanSpeed_v2(d,f):nvmlDeviceSetFanSpeed_v2(d,f,atoi(v[2])); unsigned s=0; nvmlDeviceGetFanSpeed_v2(d,f,&s); printf("GPU %s fan %u -> %s: %s (atual %u%%)\n",v[1],f,v[2],nvmlErrorString(r),s);} nvmlShutdown(); return 0; }
