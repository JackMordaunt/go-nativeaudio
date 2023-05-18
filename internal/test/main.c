#include "../../audio_windows.h"

#ifdef _MSC_VER
#define _CRT_SECURE_NO_WARNINGS
#endif

int main(int argc, char** argv) {
  if (argc < 2) { 
    printf("supply file name");
    goto done;
  }
  
  printf("press return to start media framework...\n");
  getchar();
  
  StartMediaFramework();
  
  printf("press return to decode...\n");
  getchar();
  
  const char* fname = argv[1];
  FILE* f = fopen(fname, "rb"); 

  if (f == NULL) {
    fprintf(stderr, "cannot open file '%s': %s\n", fname, strerror(errno));
    goto done;
  }

  fseek(f, 0, SEEK_END);
  const int fsize = ftell(f);
  
  fseek(f, 0, SEEK_SET);
  byte* b = malloc(fsize);

  fread(b, fsize, 1, f);
  fclose(f);
  
  Buffer* buf;
  buf = BufferNew();
  
  DecodeResult r;
  
  r = Decode(b, fsize); 
  
  free(b);
  
  if (r.Err != NULL) {
    fprintf(stderr, "decoding: %s\n", r.Err->Str);
    goto done;
  }

  printf("press return to free memory...\n");
  getchar();

  BufferFree(r.Uncompressed);
 
  printf("press return to end media framework...\n");
  getchar();
  
  EndMediaFramework();
  
  printf("press return to exit...\n");
  getchar();
  
done:
  return 0;
}

