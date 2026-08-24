import { TestBed } from '@angular/core/testing';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { HttpClient } from '@angular/common/http';
import { appConfig } from './app.config';

describe('appConfig', () => {
  let httpMock: HttpTestingController;
  let httpClient: HttpClient;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [...appConfig.providers, provideHttpClientTesting()],
    });

    httpMock = TestBed.inject(HttpTestingController);
    httpClient = TestBed.inject(HttpClient);
  });

  afterEach(() => {
    httpMock.verify();
  });

  it('should register the request ID interceptor via appConfig providers', () => {
    httpClient.get('/api/test').subscribe();

    const req = httpMock.expectOne('/api/test');
    expect(req.request.headers.has('X-Request-ID')).toBe(true);
    expect(req.request.headers.get('X-Request-ID')).toBeTruthy();
  });

  it('should generate a valid UUID v4 X-Request-ID through appConfig', () => {
    httpClient.post('/api/test', {}).subscribe();

    const req = httpMock.expectOne('/api/test');
    const requestId = req.request.headers.get('X-Request-ID');

    expect(requestId).toBeTruthy();
    expect(requestId).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i,
    );
  });

  it('should generate distinct request IDs for multiple requests through appConfig', () => {
    httpClient.get('/api/test1').subscribe();
    httpClient.get('/api/test2').subscribe();

    const req1 = httpMock.expectOne('/api/test1');
    const req2 = httpMock.expectOne('/api/test2');

    const requestId1 = req1.request.headers.get('X-Request-ID');
    const requestId2 = req2.request.headers.get('X-Request-ID');

    expect(requestId1).toBeTruthy();
    expect(requestId2).toBeTruthy();
    expect(requestId1).not.toBe(requestId2);
  });
});
