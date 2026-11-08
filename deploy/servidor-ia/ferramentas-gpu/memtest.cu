#include <cstdio>
#include <cstdlib>
#include <cuda_runtime.h>
#include <time.h>
double now(){ struct timespec t; clock_gettime(CLOCK_MONOTONIC,&t); return t.tv_sec+t.tv_nsec/1e9; }
__global__ void fill(unsigned*p,size_t n,unsigned seed){ size_t i=blockIdx.x*(size_t)blockDim.x+threadIdx.x; for(;i<n;i+=(size_t)gridDim.x*blockDim.x) p[i]=(unsigned)i*2654435761u^seed; }
__global__ void check(const unsigned*p,size_t n,unsigned seed,unsigned long long*err){ size_t i=blockIdx.x*(size_t)blockDim.x+threadIdx.x; for(;i<n;i+=(size_t)gridDim.x*blockDim.x) if(p[i]!=((unsigned)i*2654435761u^seed)) atomicAdd(err,1ULL); }
#define CK(x) do{cudaError_t r=(x); if(r!=cudaSuccess){printf("ERRO CUDA em %s: %s\n",#x,cudaGetErrorString(r));return 4;}}while(0)
int main(int c,char**v){
  int dev=atoi(v[1]); double secs=atof(v[2]); size_t mb=atol(v[3]); size_t n=mb*1024*1024/4;
  CK(cudaSetDevice(dev)); unsigned *a,*b; unsigned long long*e;
  if(cudaMalloc(&a,n*4)||cudaMalloc(&b,n*4)){printf("sem memoria\n");return 3;}
  CK(cudaMalloc(&e,8)); CK(cudaMemset(e,0,8));
  double t0=now(); long it=0; unsigned seed=12345;
  while(now()-t0<secs){
    fill<<<1024,256>>>(a,n,seed); CK(cudaGetLastError());
    for(int k=0;k<8;k++){ CK(cudaMemcpy(b,a,n*4,cudaMemcpyDeviceToDevice)); CK(cudaMemcpy(a,b,n*4,cudaMemcpyDeviceToDevice)); }
    check<<<1024,256>>>(a,n,seed,e); CK(cudaGetLastError()); CK(cudaDeviceSynchronize()); it++; seed=seed*1664525u+1013904223u;
  }
  double dt=now()-t0; double gb=(double)it*(16.0*n*4)/1e9; unsigned long long h=0; CK(cudaMemcpy(&h,e,8,cudaMemcpyDeviceToHost));
  printf("GPU %d: %ld ciclos, ~%.0f GB/s (leitura+escrita), %llu erros de memoria\n",dev,it,gb/dt,h);
  return h?5:0;
}
