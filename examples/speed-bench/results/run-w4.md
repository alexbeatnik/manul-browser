
3 rounds, 4 worker(s), 26 tests, http://127.0.0.1:60678

| | Manul | Playwright | Manul / Playwright |
|---|---:|---:|---:|
| whole run, median | 12.0s | 16.8s | 0.7× |
| whole run, fastest | 12.0s | 16.4s | |
| whole run, slowest | 12.3s | 17.1s | |
| sum of test medians | 33.8s | 51.4s | 0.7× |

| test | Manul | Playwright | Manul / Playwright |
|---|---:|---:|---:|
| cross_page_journey | 0.77s | 1.15s | 0.7× |
| form_countries | 0.84s | 1.15s | 0.7× |
| form_free_plan | 0.24s | 1.02s | 0.2× |
| form_pro_plan | 0.34s | 1.04s | 0.3× |
| form_resubmit | 0.57s | 1.05s | 0.5× |
| form_terms_gate | 0.17s | 1.01s | 0.2× |
| home_navigation | 0.73s | 1.00s | 0.7× |
| keyboard_search | 0.54s | 0.93s | 0.6× |
| login_admin | 1.05s | 1.40s | 0.7× |
| login_member | 1.08s | 1.49s | 0.7× |
| login_switch_users | 0.63s | 1.17s | 0.5× |
| login_unknown_user | 0.89s | 1.55s | 0.6× |
| login_wrong_password | 0.96s | 1.49s | 0.6× |
| mouse_everything | 0.63s | 1.02s | 0.6× |
| mouse_hover_copy | 0.32s | 0.78s | 0.4× |
| mouse_menu_and_board | 0.73s | 1.14s | 0.6× |
| quotes_cycle | 4.58s | 6.68s | 0.7× |
| quotes_mocked | 3.94s | 5.81s | 0.7× |
| shop_add_each | 0.56s | 1.02s | 0.5× |
| shop_boots | 0.55s | 0.97s | 0.6× |
| shop_from_home | 0.44s | 1.35s | 0.3× |
| shop_help_links | 0.54s | 1.05s | 0.5× |
| shop_team_edit | 0.54s | 0.83s | 0.6× |
| shop_team_remove | 0.61s | 1.10s | 0.6× |
| slow_feed | 4.43s | 5.24s | 0.8× |
| slow_report | 7.12s | 8.93s | 0.8× |

Manul, by command (all rounds):

| command | count per run | mean | share of time |
|---|---:|---:|---:|
| VERIFY | 150 | 95 ms | 42% |
| CLICK | 130 | 75 ms | 29% |
| WAIT_FOR | 3 | 1385 ms | 12% |
| NAVIGATE | 26 | 106 ms | 8% |
| DRAG | 3 | 203 ms | 2% |
| FILL | 44 | 13 ms | 2% |
| PRESS | 9 | 61 ms | 2% |
| SELECT | 6 | 67 ms | 1% |
| VERIFY_FIELD | 46 | 7 ms | 1% |
| VERIFY_PAGE | 42 | 6 ms | 1% |
| HOVER | 4 | 18 ms | 0% |
| CHECK | 9 | 8 ms | 0% |
| DOUBLE_CLICK | 3 | 16 ms | 0% |
| UNCHECK | 5 | 7 ms | 0% |
| RIGHT_CLICK | 2 | 16 ms | 0% |
| SET | 22 | 1 ms | 0% |
| TYPE | 1 | 8 ms | 0% |
| MOCK | 1 | 2 ms | 0% |
