#include "../../audio_windows.h"

#ifdef _MSC_VER
#define _CRT_SECURE_NO_WARNINGS
#endif

#if _WIN32
#  define C_RED(s)     s
#  define C_GREEN(s)   s
#else
#  define C_RED(s)     "\033[31;1m" s "\033[0m"
#  define C_GREEN(s)   "\033[32;1m" s "\033[0m"
#endif

#define TEST(s, x) \
    do { \
        if (x) { \
            printf(C_GREEN("PASS") " %s\n", s); \
            (*count_pass)++; \
        } else { \
            printf(C_RED("FAIL") " %s\n", s); \
            (*count_fail)++; \
        } \
    } while (0)

void 
TestBuffer(int *count_fail, int *count_pass) {
  
    Buffer *b = NULL;

    b = BufferNew();

    TEST("BufferNew zero len", b->Len == 0);
    TEST("BufferNew not null", b->Data != NULL);
    TEST("BufferNew default size", b->Cap == BUFFER_DEFAULT_SIZE);

    BYTE *data = malloc(1024);
    BufferWrite(b, 1024, data);
    free(data);

    TEST("BufferWrite data pointer no null", b->Data != NULL);
    TEST("BufferWrite len matches data written", b->Len == 1024);
    TEST("BufferWrite cap unchanged", b->Cap == BUFFER_DEFAULT_SIZE);
  
    for (int ii = 0; ii < 1024; ii++) {
      BYTE *data = malloc(1024);
      BufferWrite(b, 1024, data);
      free(data);
    }

    TEST("BufferWrite data pointer not null", b->Data != NULL);
    TEST("BufferWrite len matches data written", b->Len == 1024*1025);
    TEST("BufferWrite exceed cap should grow", b->Cap != BUFFER_DEFAULT_SIZE);

    BufferFree(b);
}

int
main(int argc, char** argv) {

  int count_fail = 0;
  int count_pass = 0;

  TestBuffer(&count_fail, &count_pass);
  
  printf("%d fail, %d pass\n", count_fail, count_pass);
  return count_fail != 0;
}


