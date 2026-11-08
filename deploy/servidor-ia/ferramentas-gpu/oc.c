// oc get <gpu> | oc set <gpu> <graphics|mem> <MHz>   (pstate P0; so NVML, sem instalar nada)
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <nvml.h>
static int tipo(const char*s){ return strcmp(s,"mem")==0 ? NVML_CLOCK_MEM : NVML_CLOCK_GRAPHICS; }
int main(int c,char**v){
  if(c<3){fprintf(stderr,"uso: oc get <gpu> | oc set <gpu> <graphics|mem> <MHz>\n");return 2;}
  nvmlReturn_t r=nvmlInit_v2(); if(r){fprintf(stderr,"init: %s\n",nvmlErrorString(r));return 1;}
  nvmlDevice_t d; r=nvmlDeviceGetHandleByIndex_v2(atoi(v[2]),&d); if(r){fprintf(stderr,"handle: %s\n",nvmlErrorString(r));return 1;}
  const char*nm[2]={"graphics","mem"};
  if(!strcmp(v[1],"get")){
    for(int k=0;k<2;k++){
      nvmlClockOffset_t o; memset(&o,0,sizeof o); o.version=nvmlClockOffset_v1; o.type=tipo(nm[k]); o.pstate=NVML_PSTATE_0;
      r=nvmlDeviceGetClockOffsets(d,&o);
      if(r) printf("%s: erro %s\n",nm[k],nvmlErrorString(r)); else printf("%s: offset=%d min=%d max=%d MHz\n",nm[k],o.clockOffsetMHz,o.minClockOffsetMHz,o.maxClockOffsetMHz);
    }
  } else if(!strcmp(v[1],"set")&&c>=5){
    nvmlClockOffset_t o; memset(&o,0,sizeof o); o.version=nvmlClockOffset_v1; o.type=tipo(v[3]); o.pstate=NVML_PSTATE_0; o.clockOffsetMHz=atoi(v[4]);
    r=nvmlDeviceSetClockOffsets(d,&o); printf("set %s %d: %s\n",v[3],o.clockOffsetMHz,nvmlErrorString(r)); nvmlShutdown(); return r?1:0;
  }
  nvmlShutdown(); return 0;
}
