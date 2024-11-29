-- Create the customer table
CREATE TABLE customers (
    customer_id SERIAL PRIMARY KEY,
    first_name VARCHAR(50) NOT NULL,
    last_name VARCHAR(50) NOT NULL,
    email VARCHAR(100) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create the orders table
CREATE TABLE orders (
    order_id SERIAL PRIMARY KEY,
    customer_id INT NOT NULL,
    order_date TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    amount DECIMAL(10, 2) NOT NULL,
    FOREIGN KEY (customer_id) REFERENCES customers(customer_id)
);

-- Insert sample data into customers table
INSERT INTO customers (first_name, last_name, email)
SELECT
    'FirstName' || i,
    'LastName' || i,
    'customer' || i || '@example.com'
FROM generate_series(1, 100) AS s(i);

-- Insert sample data into orders table
INSERT INTO orders (customer_id, order_date, amount)
SELECT
    (floor(random() * 100) + 1)::int,
    NOW() - (RANDOM() * INTERVAL '365 days'),
    ROUND(CAST(RANDOM() * 1000 AS NUMERIC), 2)
FROM generate_series(1, 100) AS s(i);
