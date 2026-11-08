// cb <gpu> <seg> : SGEMM 4096 (A=1,B=1 => C=4096 exato) em loop; conta elementos errados
#include <cstdio>
#include <cstdlib>
#include <cuda_runtime.h>
#include <cublas_v2.h>
#include <time.h>
double now(){ struct timespec t; clock_gettime(CLOCK_MONOTONIC,&t); return t.tv_sec+t.tv_nsec/1e9; }
__global__ void fill(float*p,size_t n,float v){ size_t i=blockIdx.x*(size_t)blockDim.x+threadIdx.x; for(;i<n;i+=(size_t)gridDim.x*blockDim.x) p[i]=v; }
__global__ void chk(const float*p,size_t n,float v,unsigned long long*e){ size_t i=blockIdx.x*(size_t)blockDim.x+threadIdx.x; for(;i<n;i+=(size_t)gridDim.x*blockDim.x) if(p[i]!=v) atomicAdd(e,1ULL); }
#define CK(x) do{cudaError_t r=(x); if(r!=cudaSuccess){printf("ERRO CUDA %s: %s\n",#x,cudaGetErrorString(r));return 4;}}while(0)
int main(int c,char**v){ int dev=atoi(v[1]); double secs=atof(v[2]); const int N=4096; size_t n=(size_t)N*N;
 CK(cudaSetDevice(dev)); float*A,*B,*C; unsigned long long*e; CK(cudaMalloc(&A,n*4)); CK(cudaMalloc(&B,n*4)); CK(cudaMalloc(&C,n*4)); CK(cudaMalloc(&e,8)); CK(cudaMemset(e,0,8));
 fill<<<512,256>>>(A,n,1.0f); fill<<<512,256>>>(B,n,1.0f); cublasHandle_t h; cublasCreate(&h); float a=1,b=0; double t0=now(); long it=0;
 while(now()-t0<secs){ cublasSgemm(h,CUBLAS_OP_N,CUBLAS_OP_N,N,N,N,&a,A,N,B,N,&b,C,N); chk<<<512,256>>>(C,n,4096.0f,e); CK(cudaDeviceSynchronize()); it++; }
 unsigned long long er=0; CK(cudaMemcpy(&er,e,8,cudaMemcpyDeviceToHost)); printf("GPU %d: %ld SGEMM, %llu erros\n",dev,it,er); return er?5:0; }
