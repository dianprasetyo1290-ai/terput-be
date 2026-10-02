# Use an official Node.js runtime as a parent image
FROM node:18 AS node

# Set working directory
WORKDIR /var/www

# Copy package.json and package-lock.json
COPY package*.json ./

# Install npm dependencies
RUN npm install

# Copy the rest of the application files
COPY . .

# Build assets using Vite
RUN npm run build

FROM php:8.4-fpm

# Set working directory
WORKDIR /var/www

# Install system dependencies
RUN apt-get update && apt-get install -y \
    git \
    curl \
    libpng-dev \
    libjpeg62-turbo-dev \
    libfreetype6-dev \
    libonig-dev \
    libzip-dev \
    zip \
    unzip \
    nginx \
    supervisor \
    libpq-dev && \
    apt-get clean && rm -rf /var/lib/apt/lists/* && \
    docker-php-ext-configure gd --with-freetype --with-jpeg && \
    docker-php-ext-install pdo pdo_pgsql pgsql mbstring exif pcntl bcmath gd zip


# Install Composer
COPY --from=composer:latest /usr/bin/composer /usr/bin/composer

# Copy existing application directory contents
COPY . /var/www

# Install Composer dependencies
RUN composer install --prefer-dist --no-scripts --no-dev --no-autoloader
RUN composer dump-autoload --optimize

# Copy existing application directory permissions
COPY --chown=www-data:www-data . /var/www

# Copy built assets from Node stage
COPY --from=node /var/www/public/build /var/www/public/build

# Setup Nginx
COPY ./docker/nginx/app.conf /etc/nginx/sites-available/default

# Copy custom PHP configuration
COPY ./docker/php/local.ini /usr/local/etc/php/conf.d/local.ini

# Setup Supervisor to manage Nginx and PHP-FPM
COPY ./docker/supervisor/supervisord.conf /etc/supervisor/conf.d/supervisord.conf

# Setup Entrypoint
COPY ./docker/entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# Change ownership and permission
RUN chown -R www-data:www-data /var/www/storage /var/www/bootstrap/cache && \
    chmod -R 775 /var/www/storage /var/www/bootstrap/cache

# Expose port 80
EXPOSE 80

# Start via entrypoint script
CMD ["/usr/local/bin/docker-entrypoint.sh"]