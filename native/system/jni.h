#ifndef JNI_H
#define JNI_H
typedef void* JNIEnv;
typedef void* jobject;
typedef void* jstring;
typedef int jint;
typedef void* JavaVM;
#define JNI_OK 0
#define JNI_VERSION_1_4 0x00010004
typedef struct { const char* name; const char* signature; void* fnPtr; } JNINativeMethod;
#define JNI_FALSE 0
#define JNI_TRUE 1
#endif