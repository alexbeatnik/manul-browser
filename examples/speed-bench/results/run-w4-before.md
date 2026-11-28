
3 rounds, 4 worker(s), 26 tests, http://127.0.0.1:55483

| | Manul | Playwright | Manul / Playwright |
|---|---:|---:|---:|
| whole run, median | 39.0s | 17.1s | 2.3× |
| whole run, fastest | 38.9s | 16.6s | |
| whole run, slowest | 39.3s | 92.4s | |
| sum of test medians | 127.3s | 51.0s | 2.5× |

| test | Manul | Playwright | Manul / Playwright |
|---|---:|---:|---:|
| cross_page_journey | 9.33s | 1.16s | 8.1× |
| form_countries | 6.22s | 1.11s | 5.6× |
| form_free_plan | 2.85s | 0.79s | 3.6× |
| form_pro_plan | 3.22s | 0.94s | 3.4× |
| form_resubmit | 4.83s | 0.94s | 5.1× |
| form_terms_gate | 2.40s | 0.80s | 3.0× |
| home_navigation | 6.41s | 1.00s | 6.4× |
| keyboard_search | 4.45s | 0.80s | 5.5× |
| login_admin | 3.69s | 1.57s | 2.3× |
| login_member | 5.83s | 1.52s | 3.8× |
| login_switch_users | 5.96s | 1.15s | 5.2× |
| login_unknown_user | 6.03s | 1.74s | 3.5× |
| login_wrong_password | 5.89s | 1.49s | 4.0× |
| mouse_everything | 4.03s | 0.97s | 4.2× |
| mouse_hover_copy | 3.34s | 0.86s | 3.9× |
| mouse_menu_and_board | 3.69s | 1.02s | 3.6× |
| quotes_cycle | 6.13s | 7.02s | 0.9× |
| quotes_mocked | 5.39s | 5.77s | 0.9× |
| shop_add_each | 4.15s | 0.97s | 4.3× |
| shop_boots | 4.07s | 1.02s | 4.0× |
| shop_from_home | 4.18s | 1.06s | 3.9× |
| shop_help_links | 4.06s | 0.79s | 5.1× |
| shop_team_edit | 4.08s | 0.80s | 5.1× |
| shop_team_remove | 4.61s | 0.94s | 4.9× |
| slow_feed | 5.62s | 5.86s | 1.0× |
| slow_report | 6.89s | 8.87s | 0.8× |

Manul, by command (all rounds):

| command | count per run | mean | share of time |
|---|---:|---:|---:|
| CLICK | 130 | 535 ms | 55% |
| NAVIGATE | 26 | 862 ms | 18% |
| FILL | 44 | 315 ms | 11% |
| VERIFY | 150 | 48 ms | 6% |
| PRESS | 9 | 309 ms | 2% |
| WAIT_FOR | 3 | 786 ms | 2% |
| SELECT | 6 | 315 ms | 1% |
| CHECK | 9 | 163 ms | 1% |
| HOVER | 4 | 330 ms | 1% |
| DOUBLE_CLICK | 3 | 332 ms | 1% |
| UNCHECK | 5 | 163 ms | 1% |
| RIGHT_CLICK | 2 | 328 ms | 1% |
| DRAG | 3 | 205 ms | 0% |
| VERIFY_FIELD | 46 | 8 ms | 0% |
| TYPE | 1 | 310 ms | 0% |
| VERIFY_PAGE | 42 | 5 ms | 0% |
| SET | 22 | 1 ms | 0% |
| MOCK | 1 | 4 ms | 0% |
