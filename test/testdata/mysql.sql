-- MySQL schema for Relica integration tests (enterprise audit: PRs #23-#27)
-- Verifies: reserved word quoting (P0), table alias quoting (P1), UPSERT, batch operations.

-- ============================================================
-- Table 1: test_reserved
-- Purpose: every column is a SQL reserved word → exercises identifier quoting (P0 fix).
-- ============================================================
CREATE TABLE IF NOT EXISTS `test_reserved` (
    `id`      INT           NOT NULL AUTO_INCREMENT PRIMARY KEY,
    `order`   INT           NOT NULL DEFAULT 0,
    `select`  VARCHAR(255)  NOT NULL DEFAULT '',
    `group`   VARCHAR(255),
    `where`   VARCHAR(255),
    `index`   INT           NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO `test_reserved` (`order`, `select`, `group`, `where`, `index`) VALUES
    (1, 'standard', 'A', 'clause1', 10),
    (2, 'vip',      'B', 'clause2', 20),
    (3, 'premium',  'A', 'clause3', 30),
    (4, 'standard', 'C', NULL,      40),
    (5, 'vip',      'B', 'clause5', 50);

-- ============================================================
-- Table 2: test_companies + test_employees
-- Purpose: table alias and JOIN quoting tests.
-- ============================================================
CREATE TABLE IF NOT EXISTS `test_companies` (
    `id`         INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
    `name`       VARCHAR(255) NOT NULL,
    `status`     VARCHAR(50)  NOT NULL DEFAULT 'active',
    `deleted_at` DATETIME     NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `test_employees` (
    `id`         INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
    `company_id` INT          NOT NULL,
    `name`       VARCHAR(255) NOT NULL,
    `role`       VARCHAR(100) NOT NULL DEFAULT 'member',
    `salary`     INT          NOT NULL DEFAULT 0,
    FOREIGN KEY (`company_id`) REFERENCES `test_companies`(`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO `test_companies` (`name`, `status`, `deleted_at`) VALUES
    ('Acme Corp',   'active',   NULL),
    ('Beta Ltd',    'active',   NULL),
    ('Gamma Inc',   'inactive', '2024-01-01 00:00:00'),
    ('Delta GmbH',  'active',   NULL),
    ('Epsilon LLC', 'inactive', '2024-06-15 00:00:00');

INSERT INTO `test_employees` (`company_id`, `name`, `role`, `salary`) VALUES
    (1, 'Alice',   'engineer', 90000),
    (1, 'Bob',     'manager',  110000),
    (2, 'Charlie', 'engineer', 85000),
    (2, 'Diana',   'lead',     95000),
    (3, 'Eve',     'engineer', 80000),
    (4, 'Frank',   'manager',  105000),
    (4, 'Grace',   'engineer', 88000),
    (4, 'Hank',    'intern',   45000);

-- ============================================================
-- Table 3: test_products
-- Purpose: UPSERT conflict resolution, batch operations.
-- ============================================================
CREATE TABLE IF NOT EXISTS `test_products` (
    `id`       INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
    `sku`      VARCHAR(100) UNIQUE NOT NULL,
    `name`     VARCHAR(255) NOT NULL,
    `price`    INT          NOT NULL DEFAULT 0,
    `category` VARCHAR(100) NOT NULL DEFAULT 'general'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO `test_products` (`sku`, `name`, `price`, `category`) VALUES
    ('SKU-001', 'Widget A',    1000, 'widgets'),
    ('SKU-002', 'Widget B',    2000, 'widgets'),
    ('SKU-003', 'Gadget X',    5000, 'gadgets'),
    ('SKU-004', 'Gadget Y',    7500, 'gadgets'),
    ('SKU-005', 'Doohickey Z',  500, 'misc');
