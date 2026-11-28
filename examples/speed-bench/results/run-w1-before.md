
5 rounds, 1 worker(s), 26 tests, http://127.0.0.1:55158

| | Manul | Playwright | Manul / Playwright |
|---|---:|---:|---:|
| whole run, median | 128.4s | 41.0s | 3.1× |
| whole run, fastest | 128.1s | 40.5s | |
| whole run, slowest | 128.9s | 41.9s | |
| sum of test medians | 127.3s | 38.9s | 3.3× |

| test | Manul | Playwright | Manul / Playwright |
|---|---:|---:|---:|
| cross_page_journey | 9.32s | 1.08s | 8.7× |
| form_countries | 6.25s | 0.56s | 11.1× |
| form_free_plan | 2.87s | 0.37s | 7.7× |
| form_pro_plan | 3.24s | 0.41s | 7.9× |
| form_resubmit | 4.84s | 0.48s | 10.0× |
| form_terms_gate | 2.42s | 0.42s | 5.8× |
| home_navigation | 6.50s | 0.90s | 7.2× |
| keyboard_search | 4.46s | 0.35s | 12.7× |
| login_admin | 3.35s | 0.57s | 5.9× |
| login_member | 5.61s | 0.52s | 10.7× |
| login_switch_users | 5.99s | 0.49s | 12.1× |
| login_unknown_user | 6.01s | 0.52s | 11.5× |
| login_wrong_password | 5.76s | 0.48s | 12.0× |
| mouse_everything | 4.04s | 0.62s | 6.5× |
| mouse_hover_copy | 3.36s | 0.43s | 7.8× |
| mouse_menu_and_board | 3.73s | 0.63s | 5.9× |
| quotes_cycle | 6.17s | 6.98s | 0.9× |
| quotes_mocked | 5.42s | 5.76s | 0.9× |
| shop_add_each | 4.19s | 0.48s | 8.8× |
| shop_boots | 4.15s | 0.46s | 9.0× |
| shop_from_home | 4.20s | 0.57s | 7.4× |
| shop_help_links | 4.13s | 0.47s | 8.8× |
| shop_team_edit | 4.09s | 0.45s | 9.2× |
| shop_team_remove | 4.63s | 0.49s | 9.4× |
| slow_feed | 5.66s | 5.68s | 1.0× |
| slow_report | 6.92s | 8.73s | 0.8× |

Manul, by command (all rounds):

| command | count per run | mean | share of time |
|---|---:|---:|---:|
| CLICK | 130 | 538 ms | 55% |
| NAVIGATE | 26 | 835 ms | 17% |
| FILL | 44 | 317 ms | 11% |
| VERIFY | 150 | 49 ms | 6% |
| PRESS | 9 | 311 ms | 2% |
| WAIT_FOR | 3 | 792 ms | 2% |
| SELECT | 6 | 317 ms | 1% |
| CHECK | 9 | 164 ms | 1% |
| HOVER | 4 | 331 ms | 1% |
| DOUBLE_CLICK | 3 | 334 ms | 1% |
| UNCHECK | 5 | 164 ms | 1% |
| RIGHT_CLICK | 2 | 330 ms | 1% |
| DRAG | 3 | 208 ms | 0% |
| VERIFY_FIELD | 46 | 10 ms | 0% |
| TYPE | 1 | 312 ms | 0% |
| VERIFY_PAGE | 42 | 7 ms | 0% |
| SET | 22 | 1 ms | 0% |
| MOCK | 1 | 4 ms | 0% |
