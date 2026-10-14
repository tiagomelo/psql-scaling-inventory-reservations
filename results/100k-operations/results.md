| Workers | Strategy | Reservations/s | p50 | p95 | p99 | max | Errors |
|--------:|----------|---------------:|----:|----:|----:|----:|-------:|
| 1 | counter | 2876 | 0.33 ms | 0.42 ms | 0.53 ms | 41.45 ms | 0 |
| 1 | skip-locked | 1997 | 0.49 ms | 0.59 ms | 0.85 ms | 12.92 ms | 0 |
| 1 | skip-locked-sharded | 2245 | 0.44 ms | 0.50 ms | 0.60 ms | 14.56 ms | 0 |
| 10 | counter | 5321 | 1.44 ms | 4.76 ms | 7.18 ms | 19.47 ms | 0 |
| 10 | skip-locked | 11295 | 0.86 ms | 1.21 ms | 1.75 ms | 5.59 ms | 0 |
| 10 | skip-locked-sharded | 12551 | 0.77 ms | 1.09 ms | 1.64 ms | 5.80 ms | 0 |
| 25 | counter | 4429 | 3.94 ms | 16.57 ms | 25.42 ms | 76.02 ms | 0 |
| 25 | skip-locked | 16171 | 1.46 ms | 2.31 ms | 3.25 ms | 12.00 ms | 0 |
| 25 | skip-locked-sharded | 18903 | 1.24 ms | 1.93 ms | 3.20 ms | 15.11 ms | 0 |
| 50 | counter | 3780 | 9.15 ms | 38.91 ms | 60.27 ms | 206.27 ms | 0 |
| 50 | skip-locked | 12718 | 3.08 ms | 9.10 ms | 16.06 ms | 59.12 ms | 0 |
| 50 | skip-locked-sharded | 25988 | 1.84 ms | 2.82 ms | 3.81 ms | 7.46 ms | 0 |
| 100 | counter | 3166 | 21.65 ms | 95.45 ms | 148.85 ms | 388.08 ms | 0 |
| 100 | skip-locked | 7913 | 5.39 ms | 50.42 ms | 117.78 ms | 547.57 ms | 0 |
| 100 | skip-locked-sharded | 27465 | 3.37 ms | 6.26 ms | 8.70 ms | 30.16 ms | 0 |
| 250 | counter | 2751 | 61.93 ms | 275.41 ms | 427.48 ms | 1097.43 ms | 0 |
| 250 | skip-locked | 3038 | 15.49 ms | 370.26 ms | 1227.53 ms | 10602.63 ms | 0 |
| 250 | skip-locked-sharded | 27955 | 7.72 ms | 18.91 ms | 30.16 ms | 64.80 ms | 0 |
| 500 | counter | 2118 | 160.42 ms | 719.34 ms | 1116.42 ms | 3265.02 ms | 0 |
| 500 | skip-locked | 1492 | 29.53 ms | 995.99 ms | 5564.07 ms | 66528.24 ms | 0 |
| 500 | skip-locked-sharded | 25363 | 15.68 ms | 48.20 ms | 75.13 ms | 186.68 ms | 0 |
| 1000 | counter | 1374 | 493.88 ms | 2213.14 ms | 3521.72 ms | 9631.74 ms | 0 |
| 1000 | skip-locked | 852 | 41.01 ms | 2704.87 ms | 26005.55 ms | 117335.06 ms | 0 |
| 1000 | skip-locked-sharded | 17388 | 37.79 ms | 174.06 ms | 308.26 ms | 994.76 ms | 0 |
