import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';

import { CreateProductRequest, Product } from '../../../core/models/product.model';
import { environment } from '../../../../environments/environment';
import { ProductApiService } from './product-api.service';

describe('ProductApiService', () => {
  let service: ProductApiService;
  let httpTesting: HttpTestingController;

  const productsUrl = `${environment.stockApiUrl}/products`;
  const productDto = {
    id: 17,
    code: 'KEYBOARD-001',
    description: 'Mechanical keyboard',
    balance: 12,
    created_at: '2026-08-20T12:00:00Z',
    updated_at: '2026-08-20T12:30:00Z',
  };
  const product: Product = {
    id: productDto.id.toString(),
    code: productDto.code,
    description: productDto.description,
    balance: productDto.balance,
    createdAt: productDto.created_at,
    updatedAt: productDto.updated_at,
  };

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [ProductApiService, provideHttpClient(), provideHttpClientTesting()],
    });

    service = TestBed.inject(ProductApiService);
    httpTesting = TestBed.inject(HttpTestingController);
  });

  afterEach(() => {
    httpTesting.verify();
  });

  it('creates a product with the exact request and maps the response', () => {
    const request: CreateProductRequest = {
      code: 'KEYBOARD-001',
      description: 'Mechanical keyboard',
      balance: 12,
    };

    service.create(request).subscribe((result) => {
      expect(result).toEqual(product);
    });

    const httpRequest = httpTesting.expectOne(productsUrl);
    expect(httpRequest.request.method).toBe('POST');
    expect(httpRequest.request.body).toEqual({
      code: 'KEYBOARD-001',
      description: 'Mechanical keyboard',
      balance: 12,
    });
    httpRequest.flush(productDto);
  });

  it('lists products and maps every response item', () => {
    const secondProductDto = {
      id: 23,
      code: 'MOUSE-001',
      description: 'Wireless mouse',
      balance: 8,
      created_at: '2026-08-20T13:00:00Z',
      updated_at: '2026-08-20T13:15:00Z',
    };

    service.list().subscribe((result) => {
      expect(result).toEqual([
        product,
        {
          id: secondProductDto.id.toString(),
          code: secondProductDto.code,
          description: secondProductDto.description,
          balance: secondProductDto.balance,
          createdAt: secondProductDto.created_at,
          updatedAt: secondProductDto.updated_at,
        },
      ]);
    });

    const httpRequest = httpTesting.expectOne(productsUrl);
    expect(httpRequest.request.method).toBe('GET');
    expect(httpRequest.request.body).toBeNull();
    httpRequest.flush([productDto, secondProductDto]);
  });

  it('gets a product by id and maps the response', () => {
    service.get(product.id).subscribe((result) => {
      expect(result).toEqual(product);
    });

    const httpRequest = httpTesting.expectOne(`${productsUrl}/${product.id}`);
    expect(httpRequest.request.method).toBe('GET');
    expect(httpRequest.request.body).toBeNull();
    httpRequest.flush(productDto);
  });
});
