2026-09-10 抓取，供 GitHub 提交源的测试当固定样本，测试不打网络。

    curl -sL https://github.com/Zakkaus/vestibule/commits.atom -o commits-vestibule.atom
    curl -sL https://github.com/gentoo-zh/overlay/commits.atom  -o commits-overlay.atom

commits-vestibule.atom  默认分支 main，20 条
commits-overlay.atom    默认分支 master，20 条（用来证明不能写死 main）

One redaction. `commits-vestibule.atom` quotes a commit whose message body names a
real Telegram supergroup. The parser never reads `<content>`, so replacing that
number with a synthetic one changes nothing these tests observe, and it keeps a
real group ID out of the repository — which is what the identity gate is for.
Original value: a `-100` supergroup ID, replaced with `-1009000010006`.
