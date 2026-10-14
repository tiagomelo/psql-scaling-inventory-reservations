| Workers | Strategy | Reservations | Reservations/s | p50 | p95 | p99 | max | Errors |
|--------:|----------|-------------:|---------------:|----:|----:|----:|----:|-------:|
| 1 | counter | 79437 | 2648 | 0.36 ms | 0.45 ms | 0.72 ms | 56.18 ms | 0 |
| 1 | skip-locked | 62925 | 2097 | 0.46 ms | 0.56 ms | 0.93 ms | 19.37 ms | 0 |
| 1 | skip-locked-sharded | 64612 | 2154 | 0.45 ms | 0.57 ms | 1.02 ms | 17.48 ms | 0 |
| 10 | counter | 165301 | 5510 | 1.40 ms | 4.54 ms | 6.84 ms | 29.88 ms | 0 |
| 10 | skip-locked | 271511 | 9050 | 1.01 ms | 1.76 ms | 2.79 ms | 17.05 ms | 0 |
| 10 | skip-locked-sharded | 354903 | 11830 | 0.80 ms | 1.23 ms | 1.87 ms | 21.59 ms | 0 |
| 25 | counter | 137401 | 4579 | 3.85 ms | 15.89 ms | 24.38 ms | 67.94 ms | 0 |
| 25 | skip-locked | 365143 | 12171 | 1.90 ms | 3.44 ms | 4.53 ms | 12.01 ms | 0 |
| 25 | skip-locked-sharded | 572740 | 19091 | 1.25 ms | 1.87 ms | 2.66 ms | 17.81 ms | 0 |
| 50 | counter | 116695 | 3889 | 8.98 ms | 37.86 ms | 58.47 ms | 140.95 ms | 0 |
| 50 | skip-locked | 324771 | 10824 | 3.86 ms | 9.91 ms | 15.91 ms | 98.43 ms | 0 |
| 50 | skip-locked-sharded | 684765 | 22824 | 2.04 ms | 3.44 ms | 5.07 ms | 28.99 ms | 0 |
| 100 | counter | 96159 | 3203 | 21.42 ms | 94.34 ms | 144.79 ms | 378.17 ms | 0 |
| 100 | skip-locked | 221748 | 7389 | 6.24 ms | 51.89 ms | 120.60 ms | 471.75 ms | 0 |
| 100 | skip-locked-sharded | 700140 | 23334 | 3.82 ms | 7.64 ms | 11.54 ms | 250.57 ms | 0 |
| 250 | counter | 78317 | 2604 | 65.01 ms | 290.53 ms | 457.67 ms | 1065.74 ms | 0 |
| 250 | skip-locked | 87638 | 2912 | 17.43 ms | 385.91 ms | 1219.85 ms | 12738.27 ms | 0 |
| 250 | skip-locked-sharded | 759882 | 25325 | 8.72 ms | 20.11 ms | 30.74 ms | 108.92 ms | 0 |
| 500 | counter | 64845 | 2148 | 159.30 ms | 705.37 ms | 1101.92 ms | 2870.94 ms | 0 |
| 500 | skip-locked | 55087 | 1763 | 17.34 ms | 960.69 ms | 5831.03 ms | 31073.37 ms | 0 |
| 500 | skip-locked-sharded | 697454 | 23241 | 17.38 ms | 52.64 ms | 83.53 ms | 318.88 ms | 0 |
| 1000 | counter | 43631 | 1434 | 472.02 ms | 2117.11 ms | 3367.42 ms | 6957.90 ms | 0 |
| 1000 | skip-locked | 33827 | 994 | 18.63 ms | 6330.53 ms | 19499.91 ms | 33855.54 ms | 0 |
| 1000 | skip-locked-sharded | 571882 | 19042 | 35.75 ms | 149.24 ms | 267.92 ms | 1466.71 ms | 0 |
