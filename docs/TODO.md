# Xend Server Future Implementations

## Auth Hardening Backlog (Future Implementation)
- Distributed/global rate limiting strategy (current in-memory + Redis mix is basic)
- Real observability/audit logs for auth events
- Robust tests (unit + integration + attack/failure cases)

## Auth Next Steps
- Add password reset flow:
  - request reset
  - verify code/token
  - set new password + revoke sessions
- Add auth audit events (structured security logs) for register/login/verify/refresh/lockout/logout
- Add tests:
  - unit tests for auth service logic
  - integration tests for register/verify/login/refresh/device routes
  - abuse/failure tests (rate limits, lockout, invalid tokens)
