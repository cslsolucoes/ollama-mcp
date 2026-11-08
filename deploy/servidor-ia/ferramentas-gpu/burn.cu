#include <cstdio>
#include <cstdlib>
#include <cuda_runtime.h>
#include <cublas_v2.h>
#include <time.h>
#include <pthread.h>

double now(){ struct timespec t; clock_gettime(CLOCK_MONOTONIC,&t); return t.tv_sec + t.tv_nsec/1e9; }

void* work(void* arg){
  int dev = *(int*)arg;
  cudaSetDevice(dev);
  int N=8192;
  float *A,*B,*C;
  cudaMalloc(&A, (size_t)N*N*sizeof(float));
  cudaMalloc(&B, (size_t)N*N*sizeof(float));
  cudaMalloc(&C, (size_t)N*N*sizeof(float));
  cudaMemset(A,1,(size_t)N*N*sizeof(float));
  cudaMemset(B,1,(size_t)N*N*sizeof(float));
  cublasHandle_t h; cublasCreate(&h);
  float a=1.0f,b=0.0f;
  int secs = atoi(getenv("BURN_SECS")?getenv("BURN_SECS"):"120");
  double t0=now();
  long it=0;
  while(now()-t0 < secs){
    cublasSgemm(h,CUBLAS_OP_N,CUBLAS_OP_N,N,N,N,&a,A,N,B,N,&b,C,N);
    cudaDeviceSynchronize();
    it++;
  }
  cublasDestroy(h); cudaFree(A);cudaFree(B);cudaFree(C);
  printf("GPU %d: %ld iteracoes\n", dev, it);
  return NULL;
}

int main(){
  int n=0; cudaGetDeviceCount(&n);
  printf("Estressando %d GPUs...\n", n);
  pthread_t th[8]; int id[8];
  for(int i=0;i<n;i++){ id[i]=i; pthread_create(&th[i],NULL,work,&id[i]); }
  for(int i=0;i<n;i++) pthread_join(th[i],NULL);
  printf("Fim.\n");
  return 0;
}
