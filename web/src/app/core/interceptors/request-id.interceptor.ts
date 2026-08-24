import { HttpInterceptorFn } from '@angular/common/http';

export const requestIdInterceptor: HttpInterceptorFn = (req, next) => {
  const reqId = crypto.randomUUID();
  const modifiedReq = req.clone({
    headers: req.headers.set('X-Request-ID', reqId)
  });
  return next(modifiedReq);
};
