import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable, map } from 'rxjs';

import { CreateProductRequest, Product } from '../../../core/models/product.model';
import { environment } from '../../../../environments/environment';

interface ProductResponseDto {
  id: number;
  code: string;
  description: string;
  balance: number;
  created_at: string;
  updated_at: string;
}

@Injectable({ providedIn: 'root' })
export class ProductApiService {
  private readonly http = inject(HttpClient);
  private readonly productsUrl = `${environment.stockApiUrl}/products`;

  create(request: CreateProductRequest): Observable<Product> {
    return this.http
      .post<ProductResponseDto>(this.productsUrl, request)
      .pipe(map((product) => this.toProduct(product)));
  }

  list(): Observable<Product[]> {
    return this.http
      .get<ProductResponseDto[]>(this.productsUrl)
      .pipe(map((products) => products.map((product) => this.toProduct(product))));
  }

  get(id: string): Observable<Product> {
    return this.http
      .get<ProductResponseDto>(`${this.productsUrl}/${encodeURIComponent(id)}`)
      .pipe(map((product) => this.toProduct(product)));
  }

  private toProduct(product: ProductResponseDto): Product {
    return {
      id: product.id.toString(),
      code: product.code,
      description: product.description,
      balance: product.balance,
      createdAt: product.created_at,
      updatedAt: product.updated_at,
    };
  }
}
