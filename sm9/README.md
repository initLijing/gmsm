## SM9 current supported functions:
1.Keys generation  
2.Sign/Verify    
3.Key Exchange  
4.Wrap/Unwrap Key  
5.Encryption/Decryption (XOR mode)

## SM9 current performance:

**SM9 Sign Benchmark**

    goos: windows
    goarch: amd64
    pkg: github.com/initLijing/gmsm/sm9
    cpu: Intel(R) Core(TM) i5-9500 CPU @ 3.00GHz
    BenchmarkSign-6   	    1344	    871597 ns/op	   35870 B/op	    1013 allocs/op


**SM9 Verify Benchmark**

    goos: windows
    goarch: amd64
    pkg: github.com/initLijing/gmsm/sm9
    cpu: Intel(R) Core(TM) i5-9500 CPU @ 3.00GHz
    BenchmarkVerify-6   	     352	   3331673 ns/op	  237676 B/op	    6283 allocs/op

**SM9 Encrypt(XOR) Benchmark**

    goos: windows
    goarch: amd64
    pkg: github.com/initLijing/gmsm/sm9
    cpu: Intel(R) Core(TM) i5-9500 CPU @ 3.00GHz
    BenchmarkEncrypt-6   	    1120	    971188 ns/op	   38125 B/op	    1036 allocs/op

**SM9 Decrypt(XOR) Benchmark**

    goos: windows
    goarch: amd64
    pkg: github.com/initLijing/gmsm/sm9
    cpu: Intel(R) Core(TM) i5-9500 CPU @ 3.00GHz
    BenchmarkDecrypt-6   	     507	   2345492 ns/op	  202360 B/op	    5228 allocs/op

**SM9 Generate User Sign Private Key Benchmark**

    goos: windows
    goarch: amd64
    pkg: github.com/initLijing/gmsm/sm9
    cpu: Intel(R) Core(TM) i5-9500 CPU @ 3.00GHz
    BenchmarkGenerateSignPrivKey-6   	    8078	    147638 ns/op	    3176 B/op	      47 allocs/op

**SM9 Generate User Encrypt Private Key Benchmark**

    goos: windows
    goarch: amd64
    pkg: github.com/initLijing/gmsm/sm9
    cpu: Intel(R) Core(TM) i5-9500 CPU @ 3.00GHz
    BenchmarkGenerateEncryptPrivKey-6   	    3445	    326796 ns/op	    3433 B/op	      47 allocs/op

To further improve `Verify()/Decrypt()` performance, need to improve `Pair()` method performance.
