
5 rounds, 1 worker(s), 26 tests, http://127.0.0.1:60373

| | Manul | Playwright | Manul / Playwright |
|---|---:|---:|---:|
| whole run, median | 32.2s | 40.9s | 0.8× |
| whole run, fastest | 32.0s | 40.3s | |
| whole run, slowest | 32.2s | 41.0s | |
| sum of test medians | 31.1s | 38.9s | 0.8× |

| test | Manul | Playwright | Manul / Playwright |
|---|---:|---:|---:|
| cross_page_journey | 0.68s | 1.06s | 0.6× |
| form_countries | 0.82s | 0.57s | 1.5× |
| form_free_plan | 0.21s | 0.37s | 0.6× |
| form_pro_plan | 0.29s | 0.41s | 0.7× |
| form_resubmit | 0.54s | 0.49s | 1.1× |
| form_terms_gate | 0.15s | 0.42s | 0.4× |
| home_navigation | 0.45s | 0.90s | 0.5× |
| keyboard_search | 0.52s | 0.36s | 1.4× |
| login_admin | 0.38s | 0.56s | 0.7× |
| login_member | 0.48s | 0.51s | 0.9× |
| login_switch_users | 0.56s | 0.51s | 1.1× |
| login_unknown_user | 0.51s | 0.52s | 1.0× |
| login_wrong_password | 0.48s | 0.46s | 1.0× |
| mouse_everything | 0.63s | 0.61s | 1.0× |
| mouse_hover_copy | 0.30s | 0.43s | 0.7× |
| mouse_menu_and_board | 0.76s | 0.65s | 1.2× |
| quotes_cycle | 4.58s | 6.99s | 0.7× |
| quotes_mocked | 3.89s | 5.77s | 0.7× |
| shop_add_each | 0.55s | 0.47s | 1.2× |
| shop_boots | 0.54s | 0.45s | 1.2× |
| shop_from_home | 0.39s | 0.57s | 0.7× |
| shop_help_links | 0.55s | 0.47s | 1.2× |
| shop_team_edit | 0.53s | 0.46s | 1.1× |
| shop_team_remove | 0.63s | 0.48s | 1.3× |
| slow_feed | 4.48s | 5.71s | 0.8× |
| slow_report | 7.21s | 8.73s | 0.8× |

Manul, by command (all rounds):

| command | count per run | mean | share of time |
|---|---:|---:|---:|
| VERIFY | 150 | 96 ms | 46% |
| CLICK | 130 | 70 ms | 29% |
| WAIT_FOR | 3 | 1399 ms | 14% |
| NAVIGATE | 26 | 28 ms | 2% |
| DRAG | 3 | 205 ms | 2% |
| PRESS | 9 | 60 ms | 2% |
| SELECT | 6 | 63 ms | 1% |
| FILL | 44 | 7 ms | 1% |
| VERIFY_FIELD | 46 | 6 ms | 1% |
| VERIFY_PAGE | 42 | 4 ms | 0% |
| HOVER | 4 | 16 ms | 0% |
| DOUBLE_CLICK | 3 | 21 ms | 0% |
| CHECK | 9 | 5 ms | 0% |
| UNCHECK | 5 | 7 ms | 0% |
| RIGHT_CLICK | 2 | 16 ms | 0% |
| SET | 22 | 0 ms | 0% |
| TYPE | 1 | 4 ms | 0% |
| MOCK | 1 | 2 ms | 0% |
